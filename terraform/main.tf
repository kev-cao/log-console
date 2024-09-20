resource "google_service_account" "vault_sa" {
  account_id   = "vault-sa"
  display_name = "Vault Service Account"
  description  = "Service account for Vault."
}

resource "google_kms_key_ring" "vault_keyring" {
  name     = "vault-keyring"
  location = var.gcloud_region
}

resource "google_kms_crypto_key" "unseal_key" {
  name            = "unseal-key"
  key_ring        = google_kms_key_ring.vault_keyring.id
  rotation_period = "7776000s"
  lifecycle {
    prevent_destroy = true
  }
}

resource "google_kms_crypto_key_iam_member" "vault_encrypter_decrypter_policy" {
  crypto_key_id = google_kms_crypto_key.unseal_key.id
  role          = "roles/cloudkms.cryptoKeyEncrypterDecrypter"
  member        = "serviceAccount:${google_service_account.vault_sa.email}"
}

resource "google_kms_crypto_key_iam_member" "vault_viewer_policy" {
  crypto_key_id = google_kms_crypto_key.unseal_key.id
  role          = "roles/cloudkms.viewer"
  member        = "serviceAccount:${google_service_account.vault_sa.email}"
}
