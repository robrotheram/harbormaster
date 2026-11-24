package api

import (
	"errors"
	"strings"

	"agent-workbench/internal/forge"
	"agent-workbench/internal/subscriptions"
)

// CredentialKeys configures a controlplane-only encryption keyring. Untagged
// records predate key IDs and require an explicitly configured legacy key.
type CredentialKeys struct {
	Active string            `json:"active"`
	Keys   map[string]string `json:"keys"`
	Legacy string            `json:"legacy,omitempty"`
}

func (k CredentialKeys) Validate(controlToken string) error {
	if k.Keys[k.Active] == "" || len(k.Keys) > 16 {
		return errors.New("credential encryption keyring requires an active key")
	}
	for id, key := range k.Keys {
		if id == "" || len(id) > 32 || strings.ContainsAny(id, ": \r\n\t") || len(key) < 32 ||
			strings.ContainsAny(key, " \r\n\t") || key == controlToken {
			return errors.New("invalid credential encryption key")
		}
	}
	return nil
}

func (s *Server) credentialWriteKey() (key, prefix string) {
	k := s.cfg.CredentialKeys
	if k.Active == "" {
		if s.cfg.RequireScopedCredentials {
			return "", ""
		}
		return s.cfg.Token, ""
	}
	return k.Keys[k.Active], k.Active + ":"
}

func (s *Server) credentialReadKey(ciphertext string) (key, payload string) {
	id, body, tagged := strings.Cut(ciphertext, ":")
	if !tagged {
		if s.cfg.CredentialKeys.Legacy != "" {
			return s.cfg.CredentialKeys.Legacy, ciphertext
		}
		if !s.cfg.RequireScopedCredentials && s.cfg.CredentialKeys.Active == "" {
			return s.cfg.Token, ciphertext
		}
		return "", ciphertext
	}
	return s.cfg.CredentialKeys.Keys[id], body
}

func (s *Server) sealSubscription(owner, id string, credential subscriptions.Credential) (string, error) {
	key, prefix := s.credentialWriteKey()
	sealed, err := subscriptions.Seal(key, subscriptions.Binding(owner, id), credential)
	return prefix + sealed, err
}

func (s *Server) openSubscription(owner, id, ciphertext string) (subscriptions.Credential, error) {
	key, payload := s.credentialReadKey(ciphertext)
	return subscriptions.Open(key, subscriptions.Binding(owner, id), payload)
}

func (s *Server) sealForgeCredential(host forge.Host) (string, error) {
	key, prefix := s.credentialWriteKey()
	sealed, err := forge.SealToken(host, key)
	return prefix + sealed, err
}

func (s *Server) openForgeCredential(host forge.Host, ciphertext string) (string, error) {
	key, payload := s.credentialReadKey(ciphertext)
	return forge.OpenToken(host, key, payload)
}
