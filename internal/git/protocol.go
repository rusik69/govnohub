package gitstore

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// WriteServicePacket writes the smart HTTP service announcement (pkt-line framed).
func WriteServicePacket(w io.Writer, service string) error {
	line := fmt.Sprintf("# service=%s\n", service)
	pkt := formatPktLine(line)
	if _, err := w.Write(pkt); err != nil {
		return err
	}
	_, err := w.Write([]byte("0000"))
	return err
}

func formatPktLine(data string) []byte {
	n := len(data) + 4
	return []byte(fmt.Sprintf("%04x%s", n, data))
}

// AdvertiseRefs runs git *-pack --advertise-refs and writes pkt-line output.
func AdvertiseRefs(repoPath, service string, w io.Writer) error {
	cmd := exec.Command("git", packCommand(service), "--stateless-rpc", "--advertise-refs", repoPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return err
	}
	_, err := w.Write(out.Bytes())
	return err
}

func packCommand(service string) string {
	switch service {
	case "git-receive-pack", "receive-pack":
		return "receive-pack"
	default:
		return "upload-pack"
	}
}

// ServiceFromQuery returns the git service name from ?service= query param.
func ServiceFromQuery(service string) string {
	switch service {
	case "git-upload-pack":
		return "git-upload-pack"
	case "git-receive-pack":
		return "git-receive-pack"
	default:
		return ""
	}
}

// AdvertisementContentType returns the Content-Type for info/refs responses.
func AdvertisementContentType(service string) string {
	switch service {
	case "git-receive-pack":
		return "application/x-git-receive-pack-advertisement"
	default:
		return "application/x-git-upload-pack-advertisement"
	}
}

// RequestContentType returns expected Content-Type for pack RPC requests.
func RequestContentType(service string) string {
	switch service {
	case "git-receive-pack":
		return "application/x-git-receive-pack-request"
	default:
		return "application/x-git-upload-pack-request"
	}
}

// ResultContentType returns Content-Type for pack RPC responses.
func ResultContentType(service string) string {
	switch service {
	case "git-receive-pack":
		return "application/x-git-receive-pack-result"
	default:
		return "application/x-git-upload-pack-result"
	}
}

// NormalizeService maps path segments to canonical service names.
func NormalizeService(segment string) string {
	switch strings.TrimPrefix(segment, "git-") {
	case "receive-pack":
		return "git-receive-pack"
	case "upload-pack":
		return "git-upload-pack"
	default:
		return segment
	}
}
