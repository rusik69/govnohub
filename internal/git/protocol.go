package gitstore

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// RefUpdate represents a single reference update from a receive-pack request.
type RefUpdate struct {
	OldSHA string `json:"old_sha"`
	NewSHA string `json:"new_sha"`
	Ref    string `json:"ref"`
}

// ParseRefUpdates parses the ref update pkt-lines from a git-receive-pack request body.
// It returns the ref updates and the number of bytes consumed (up to and including
// the flush-pkt). The remaining bytes are the packfile data.
func ParseRefUpdates(data []byte) ([]RefUpdate, error) {
	var refs []RefUpdate
	r := bytes.NewReader(data)
	for r.Len() > 0 {
		var hexLen [4]byte
		if _, err := io.ReadFull(r, hexLen[:]); err != nil {
			break
		}
		n, err := strconv.ParseInt(string(hexLen[:]), 16, 32)
		if err != nil || n <= 0 {
			break
		}
		if n == 4 {
			// flush-pkt (0004 means empty line, but 0000 is the actual flush)
			// 0000 is handled above (n=0)
			continue
		}
		if n < 4 {
			break
		}
		line := make([]byte, n-4)
		if _, err := io.ReadFull(r, line); err != nil {
			break
		}
		// Ref update format: "<old-sha> <new-sha> <refname>\0<caps>\n"
		// Strip trailing newline
		line = bytes.TrimRight(line, "\n")
		parts := strings.SplitN(string(line), " ", 3)
		if len(parts) < 3 {
			continue
		}
		oldSHA := parts[0]
		newSHA := parts[1]
		ref := parts[2]
		// Remove capabilities after null byte
		if idx := strings.IndexByte(ref, 0); idx >= 0 {
			ref = ref[:idx]
		}
		if len(oldSHA) != 40 || len(newSHA) != 40 {
			continue
		}
		refs = append(refs, RefUpdate{
			OldSHA: oldSHA,
			NewSHA: newSHA,
			Ref:    ref,
		})
	}
	return refs, nil
}

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
