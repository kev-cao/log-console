# Allow managing leases
path "sys/leases/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Manage auth methods broadly across Vault
path "auth/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Create, update, and delete auth methods
path "sys/auth/*"
{
  capabilities = ["create", "update", "delete", "sudo"]
}

# List auth methods
path "sys/auth"
{
  capabilities = ["read"]
}

# List existing policies
path "sys/policies/acl"
{
  capabilities = ["read","list"]
}

# Create and manage ACL policies
path "sys/policies/acl/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Manage secret engines
path "sys/mounts/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# List existing secret engines.
path "sys/mounts"
{
  capabilities = ["read"]
}

# Read health checks
path "sys/health"
{
  capabilities = ["read", "sudo"]
}

#################################
# KV permissions
#################################
path "kv/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# KV v2 specific: Allow access to metadata endpoints
path "kv/+/metadata/*" {
  capabilities = ["list", "read"]
}

# KV v2 specific: Allow access to data endpoints
path "kv/+/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

#################################
# DB permissions
#################################
path "database/*"
{
	capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Manage database connection configurations
path "database/+/config/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
}

# Manage roles for generating credentials
path "database/+/roles/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
}

# Generate and view credentials
path "database/+/creds/*" {
    capabilities = ["read", "list"]
}

#################################
# PKI permissions
#################################
path "pki/*"
{
  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
}

# Manage roles for issuing certificates
path "pki/roles/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
}

# Issue certificates
path "pki/intermediate/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
}

# Manage CRLs 
path "pki/config/*" {
    capabilities = ["create", "read", "update", "delete", "list"]
}