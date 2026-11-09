disable_mlock = true
ui = false
storage "file" {
  path = "/vault/file"
}
listener "tcp" {
  address = "0.0.0.0:8200"
  tls_cert_file = "/vault/tls/cert.pem"
  tls_key_file = "/run/secrets/vault_tls_key"
}
