package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

var gitSSHCmd = regexp.MustCompile(`^git-(upload-pack|receive-pack) '?([^']+)'?$`)

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
		cmd := strings.TrimSpace(string(req.Payload[4:]))
		if req.WantReply {
			req.Reply(true, nil)
		}
		s.execGitSSH(context.Background(), cmd, userID, username, channel, channel)
		return
	}
}

func (s *server) execGitSSH(ctx context.Context, cmd string, userID uuid.UUID, username string, r io.Reader, w io.Writer) {
	m := gitSSHCmd.FindStringSubmatch(cmd)
	if len(m) != 3 {
		fmt.Fprintf(w, "err: unknown git command\n")
		return
	}
	service := m[1]
	repoPath := strings.TrimSuffix(m[2], ".git")
	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 {
		fmt.Fprintf(w, "err: invalid repository path\n")
		return
	}
	owner, name := parts[0], parts[1]

	repository, err := s.repos.GetByFullName(ctx, owner, name)
	if err != nil {
		fmt.Fprintf(w, "err: repository not found\n")
		return
	}
	okAccess, _ := s.repos.CanAccess(ctx, repository.ID, userID, "read")
	if !okAccess {
		fmt.Fprintf(w, "err: forbidden\n")
		return
	}
	if !s.git.Exists(owner, name) {
		fmt.Fprintf(w, "err: repository not found\n")
		return
	}

	switch service {
	case "upload-pack":
		if err := s.git.UploadPackSSH(owner, name, r, w); err != nil {
			log.Printf("ssh upload-pack %s/%s: %v", owner, name, err)
		}
	case "receive-pack":
		canWrite, _ := s.repos.CanAccess(ctx, repository.ID, userID, "write")
		if !canWrite {
			fmt.Fprintf(w, "err: forbidden\n")
			return
		}
		before, err := s.git.ListBranchSHAs(owner, name)
		if err != nil {
			log.Printf("ssh list branches before push %s/%s: %v", owner, name, err)
			return
		}
		if err := s.git.ReceivePackSSH(owner, name, r, w); err != nil {
			log.Printf("ssh receive-pack %s/%s: %v", owner, name, err)
			return
		}
		s.afterPush(ctx, repository, owner, name, username, before)
	default:
		fmt.Fprintf(w, "err: unknown service\n")
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
