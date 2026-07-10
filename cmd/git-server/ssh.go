package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

func (s *server) startSSH(addr, hostKeyPath string) {
	if addr == "" {
		return
	}
	hostKey, err := loadOrGenerateHostKey(hostKeyPath)
	if err != nil {
		log.Fatalf("ssh host key: %v", err)
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: s.sshPublicKeyCallback,
	}
	cfg.AddHostKey(hostKey)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("ssh listen %s: %v", addr, err)
	}
	log.Printf("git-server ssh listening on %s", addr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Printf("ssh accept: %v", err)
				continue
			}
			go s.handleSSHConn(conn, cfg)
		}
	}()
}

func (s *server) sshPublicKeyCallback(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	userID, username, err := s.auth.LookupUserBySSHPublicKey(context.Background(), key)
	if err != nil {
		return nil, fmt.Errorf("unauthorized")
	}
	return &ssh.Permissions{
		Extensions: map[string]string{
			"user-id":  userID.String(),
			"username": username,
		},
	}, nil
}

func (s *server) handleSSHConn(raw net.Conn, cfg *ssh.ServerConfig) {
	defer raw.Close()
	sshConn, chans, reqs, err := ssh.NewServerConn(raw, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()

	userID, err := uuid.Parse(sshConn.Permissions.Extensions["user-id"])
	if err != nil {
		return
	}
	username := sshConn.Permissions.Extensions["username"]

	go ssh.DiscardRequests(reqs)
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go s.handleSSHSession(channel, requests, userID, username)
	}
}

func (s *server) handleSSHSession(channel ssh.Channel, requests <-chan *ssh.Request, userID uuid.UUID, username string) {
	defer channel.Close()
	for req := range requests {
		if req.Type != "exec" {
			if req.WantReply {
				req.Reply(false, nil)
			}
			continue
		}
		cmd := parseExecCommand(req.Payload)
		if req.WantReply {
			req.Reply(true, nil)
		}
		// Drain remaining session requests while git runs; Git 2.54+ may send
		// env/pty requests that block the channel if left unread.
		go func() {
			for r := range requests {
				if r.WantReply {
					r.Reply(false, nil)
				}
			}
		}()
		status := s.execGitSSH(context.Background(), cmd, userID, username, channel, channel)
		sendExitStatus(channel, status)
		return
	}
}

func sendExitStatus(ch ssh.Channel, status uint32) {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], status)
	_, _ = ch.SendRequest("exit-status", false, buf[:])
}

func parseGitSSHCommand(cmd string) (service, repoPath string, ok bool) {
	cmd = strings.TrimSpace(cmd)
	switch {
	case strings.HasPrefix(cmd, "git-upload-pack"):
		service = "upload-pack"
	case strings.HasPrefix(cmd, "git-receive-pack"):
		service = "receive-pack"
	default:
		return "", "", false
	}
	i := strings.LastIndex(cmd, " ")
	if i < 0 {
		return "", "", false
	}
	repoPath = strings.Trim(strings.TrimSuffix(strings.Trim(cmd[i+1:], `"'`), ".git"), "/")
	return service, repoPath, repoPath != ""
}

func parseExecCommand(payload []byte) string {
	var cmd string
	if err := ssh.Unmarshal(payload, &cmd); err == nil {
		return strings.TrimSpace(cmd)
	}
	if len(payload) > 4 {
		return strings.TrimSpace(string(payload[4:]))
	}
	return strings.TrimSpace(string(payload))
}

func (s *server) execGitSSH(ctx context.Context, cmd string, userID uuid.UUID, username string, r io.Reader, w io.Writer) uint32 {
	service, repoPath, ok := parseGitSSHCommand(cmd)
	if !ok {
		log.Printf("ssh: unknown command %q", cmd)
		return 127
	}
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 {
		log.Printf("ssh: invalid repository path %q from %q", repoPath, cmd)
		return 1
	}
	owner, name := parts[0], parts[1]

	repository, err := s.repos.GetByFullName(ctx, owner, name)
	if err != nil {
		log.Printf("ssh: repository not found %s/%s", owner, name)
		return 1
	}
	okAccess, _ := s.repos.CanAccess(ctx, repository.ID, userID, "read")
	if !okAccess {
		log.Printf("ssh: forbidden read %s/%s", owner, name)
		return 1
	}
	if !s.git.Exists(owner, name) {
		log.Printf("ssh: repository missing on disk %s/%s", owner, name)
		return 1
	}

	switch service {
	case "upload-pack":
		if err := s.git.UploadPackSSH(owner, name, r, w); err != nil {
			log.Printf("ssh upload-pack %s/%s: %v", owner, name, err)
			return 1
		}
		return 0
	case "receive-pack":
		canWrite, _ := s.repos.CanAccess(ctx, repository.ID, userID, "write")
		if !canWrite {
			log.Printf("ssh: forbidden write %s/%s", owner, name)
			return 1
		}
		before, err := s.git.ListBranchSHAs(owner, name)
		if err != nil {
			log.Printf("ssh list branches before push %s/%s: %v", owner, name, err)
			return 1
		}
		if err := s.git.ReceivePackSSH(owner, name, r, w); err != nil {
			log.Printf("ssh receive-pack %s/%s: %v", owner, name, err)
			return 1
		}
		s.afterPush(ctx, repository, owner, name, username, before)
		return 0
	default:
		log.Printf("ssh: unknown service %q", service)
		return 1
	}
}

func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			return ssh.ParsePrivateKey(data)
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, err
	}
	if path != "" {
		pemKey := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
		if err := os.WriteFile(path, pemKey, 0o600); err != nil {
			log.Printf("ssh: could not persist host key to %s: %v", path, err)
		}
	}
	return signer, nil
}
