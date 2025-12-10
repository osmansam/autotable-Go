# Audit Logs Authorization Configuration Examples

# Example 1: Default configuration (admin only)

AUDIT_LOGS_AUTHORIZATION_ENABLED=true
AUDIT_LOGS_AUTHORIZED_ROLES=admin

# Example 2: Multiple roles allowed

# AUDIT_LOGS_AUTHORIZATION_ENABLED=true

# AUDIT_LOGS_AUTHORIZED_ROLES=admin,auditor,superadmin

# Example 3: Disable authorization (any authenticated user can access)

# AUDIT_LOGS_AUTHORIZATION_ENABLED=false

# Example 4: Allow specific custom roles

# AUDIT_LOGS_AUTHORIZATION_ENABLED=true

# AUDIT_LOGS_AUTHORIZED_ROLES=security_officer,compliance_manager
