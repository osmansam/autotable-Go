# Multi-Tenancy Authentication System Guide

## 📋 Overview

This system implements a complete multi-tenancy architecture where:

- **Users** are global and can belong to multiple **Tenants** (organizations)
- **Tenants** contain multiple **Projects**
- **Projects** contain the actual work (containers, pages, dynamic data)
- Authentication is separate for tenant users vs. dynamic route users

---

## 🏗️ Core Entities

### 1. User (Global)

**Collection:** `users`

A person with email/password who can belong to multiple tenants.

```json
{
  "_id": "ObjectId",
  "email": "john@example.com",
  "name": "John Doe",
  "password": "hashed_password",
  "isActive": true,
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Key Points:**

- ✅ One user can be in multiple tenants
- ✅ Global authentication across all tenants
- ✅ Email must be unique

---

### 2. Tenant (Organization)

**Collection:** `tenants`

An organization/company that contains projects and team members.

```json
{
  "_id": "ObjectId",
  "name": "Acme Corp",
  "slug": "acme-corp",
  "ownerUserId": "ObjectId (User)",
  "isActive": true,
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Key Points:**

- ✅ Like a Slack workspace or GitHub organization
- ✅ Has one owner (creator)
- ✅ Slug must be unique (used in URLs)

---

### 3. Project

**Collection:** `projects`

A project within a tenant where actual work happens.

```json
{
  "_id": "ObjectId",
  "tenantId": "ObjectId (Tenant)",
  "name": "Mobile App",
  "slug": "mobile-app",
  "isActive": true,
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Key Points:**

- ✅ Belongs to ONE tenant
- ✅ Contains containers, pages, and data
- ✅ Users need project membership to access

---

### 4. TenantMembership

**Collection:** `tenant_memberships`

Links a user to a tenant with tenant-level roles.

```json
{
  "_id": "ObjectId",
  "tenantId": "ObjectId",
  "userId": "ObjectId",
  "roles": ["tenant_owner"],
  "status": "active",
  "createdBy": "ObjectId",
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Tenant Roles:**

- `tenant_owner` - Full control, can delete tenant
- `tenant_admin` - Manage members, projects, settings
- `tenant_billing` - Access billing only
- `tenant_auditor` - Read-only access to audit logs

---

### 5. ProjectMembership

**Collection:** `project_memberships`

Links a user to a project with project-level roles.

```json
{
  "_id": "ObjectId",
  "tenantId": "ObjectId",
  "projectId": "ObjectId",
  "userId": "ObjectId",
  "roles": ["project_developer"],
  "status": "active",
  "createdBy": "ObjectId",
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Project Roles:**

- `project_admin` - Full project control
- `project_developer` - Create/edit containers, pages
- `project_editor` - Edit content, pages
- `project_viewer` - Read-only access
- `project_support` - Support access

---

### 6. Invite

**Collection:** `invites`

Invitation to join a tenant or project.

```json
{
  "_id": "ObjectId",
  "scope": "project",
  "tenantId": "ObjectId",
  "projectId": "ObjectId",
  "email": "alice@example.com",
  "rolesToAssign": ["project_developer"],
  "tokenHash": "hashed_token",
  "status": "invited",
  "expiresAt": "timestamp",
  "createdBy": "ObjectId",
  "acceptedBy": "ObjectId",
  "acceptedAt": "timestamp",
  "createdAt": "timestamp",
  "updatedAt": "timestamp"
}
```

**Invite Scopes:**

- `tenant` - Join tenant only
- `project` - Join specific project (and tenant implicitly)

---

## 🔐 Authentication Flow

### Scenario: Owner Invites Developer

#### **Step 1: Owner Registers & Creates Tenant**

```bash
POST /api/tenant/auth/register
Content-Type: application/json

{
  "email": "john@acme.com",
  "password": "SecurePass123!",
  "name": "John Doe",
  "tenantName": "Acme Corp",
  "tenantSlug": "acme-corp"
}
```

**Creates:**

- ✅ User (john@acme.com)
- ✅ Tenant (Acme Corp)
- ✅ TenantMembership (John → Acme Corp as `tenant_owner`)

**Response:**

```json
{
  "status": 201,
  "message": "User and tenant created successfully",
  "data": {
    "user": { "id": "...", "email": "john@acme.com", "name": "John Doe" },
    "tenant": { "id": "...", "name": "Acme Corp", "slug": "acme-corp" },
    "accessToken": "eyJhbGc...",
    "refreshToken": "eyJhbGc..."
  }
}
```

---

#### **Step 2: Owner Logs In (Subsequent Visits)**

```bash
POST /api/tenant/auth/login
Content-Type: application/json

{
  "email": "john@acme.com",
  "password": "SecurePass123!",
  "tenantId": "optional-if-user-has-multiple-tenants"
}
```

**Response:**

```json
{
  "status": 200,
  "message": "Login successful",
  "data": {
    "accessToken": "eyJhbGc...",
    "refreshToken": "eyJhbGc...",
    "user": { "id": "...", "email": "john@acme.com", "name": "John Doe" },
    "tenant": { "id": "...", "name": "Acme Corp" },
    "roles": ["tenant_owner"],
    "allTenants": [...]
  }
}
```

---

#### **Step 3: Owner Creates Project** _(TODO: Implement)_

```bash
POST /api/tenant/projects
Authorization: Bearer <tenant-scoped-token>
Content-Type: application/json

{
  "name": "Mobile App",
  "slug": "mobile-app"
}
```

**Should Create:**

- ✅ Project (Mobile App) under Acme Corp
- ✅ ProjectMembership (John → Mobile App as `project_admin`)

---

#### **Step 4: Owner Invites Developer** _(TODO: Implement)_

```bash
POST /api/tenant/invites
Authorization: Bearer <tenant-scoped-token>
Content-Type: application/json

{
  "email": "alice@developer.com",
  "scope": "project",
  "projectId": "67890...",
  "rolesToAssign": ["project_developer"]
}
```

**Should Create:**

- ✅ Invite record with token
- ✅ Send email to alice@developer.com with invite link

---

#### **Step 5: Developer Registers (New User)**

```bash
POST /api/tenant/auth/register
Content-Type: application/json

{
  "email": "alice@developer.com",
  "password": "AlicePass456!",
  "name": "Alice Developer"
}
```

**Note:** Developer doesn't create a tenant, just a user account.

**Alternative:** If Alice already has an account, she just logs in:

```bash
POST /api/tenant/auth/login
Content-Type: application/json

{
  "email": "alice@developer.com",
  "password": "AlicePass456!"
}
```

---

#### **Step 6: Developer Accepts Invite** _(TODO: Implement)_

```bash
POST /api/tenant/invites/accept
Authorization: Bearer <token>
Content-Type: application/json

{
  "inviteToken": "abc123..."
}
```

**Should Create:**

- ✅ TenantMembership (Alice → Acme Corp)
- ✅ ProjectMembership (Alice → Mobile App as `project_developer`)

---

#### **Step 7: Developer Switches to Project**

```bash
POST /api/tenant/auth/switch-project
Authorization: Bearer <tenant-token>
Content-Type: application/json

{
  "projectId": "67890..."
}
```

**Response:**

```json
{
  "status": 200,
  "message": "Switched to project context",
  "data": {
    "accessToken": "eyJhbGc...", // NEW TOKEN with project scope
    "refreshToken": "eyJhbGc...",
    "project": { "id": "...", "name": "Mobile App" },
    "roles": ["project_developer"]
  }
}
```

---

#### **Step 8: Developer Uses Container/Page APIs**

Now Alice can create containers and pages!

```bash
POST /api/v1/container
Authorization: Bearer <project-scoped-token>
Content-Type: application/json

{
  "schemaName": "users",
  "fields": [...]
}
```

✅ **Allowed** - Alice has `project_developer` role in project scope

```bash
POST /api/v1/dynamic/users
Authorization: Bearer <dynamic-route-token>

{...}
```

❌ **Not Allowed** - Dynamic routes use different auth (old system)

---

## 🎫 Token Types

### Tenant-Scoped Token

```json
{
  "user_id": "123",
  "email": "alice@developer.com",
  "tenant_id": "acme-corp-id",
  "project_id": "", // Empty!
  "roles": ["tenant_admin"],
  "role_scope": "tenant",
  "exp": 1234567890
}
```

**Can Access:**

- Tenant management
- Create/list projects
- Invite users
- View members

---

### Project-Scoped Token

```json
{
  "user_id": "123",
  "email": "alice@developer.com",
  "tenant_id": "acme-corp-id",
  "project_id": "mobile-app-id", // Filled!
  "roles": ["project_developer"],
  "role_scope": "project",
  "exp": 1234567890
}
```

**Can Access:**

- Container routes (`/api/v1/container`)
- Page routes (`/api/v1/page`)
- Project-specific data

---

## 🛡️ Authorization Matrix

### Container Routes (`/api/v1/container`)

| Action           | Required Roles                       |
| ---------------- | ------------------------------------ |
| Create Container | `project_admin`, `project_developer` |
| View Containers  | Any project member                   |
| Update Container | `project_admin`, `project_developer` |
| Delete Container | `project_admin` only                 |
| Reset Redis      | `project_admin` only                 |

### Page Routes (`/api/v1/page`)

| Action      | Required Roles                                         |
| ----------- | ------------------------------------------------------ |
| Create Page | `project_admin`, `project_developer`, `project_editor` |
| View Pages  | Any project member                                     |
| Update Page | `project_admin`, `project_developer`, `project_editor` |
| Delete Page | `project_admin` only                                   |

---

## 🔄 API Endpoints

### ✅ Implemented

| Method | Endpoint                          | Description                   |
| ------ | --------------------------------- | ----------------------------- |
| POST   | `/api/tenant/auth/register`       | Register user + create tenant |
| POST   | `/api/tenant/auth/login`          | Login to tenant               |
| POST   | `/api/tenant/auth/refresh`        | Refresh access token          |
| POST   | `/api/tenant/auth/logout`         | Logout (audit only)           |
| GET    | `/api/tenant/auth/me`             | Get current user info         |
| POST   | `/api/tenant/auth/switch-project` | Switch to project scope       |

---

### ⏳ TODO: Still Need to Implement

#### Project Management

| Method | Endpoint                   | Description             |
| ------ | -------------------------- | ----------------------- |
| POST   | `/api/tenant/projects`     | Create project          |
| GET    | `/api/tenant/projects`     | List projects in tenant |
| GET    | `/api/tenant/projects/:id` | Get project details     |
| PATCH  | `/api/tenant/projects/:id` | Update project          |
| DELETE | `/api/tenant/projects/:id` | Delete project          |

#### Invite System

| Method | Endpoint                     | Description   |
| ------ | ---------------------------- | ------------- |
| POST   | `/api/tenant/invites`        | Create invite |
| GET    | `/api/tenant/invites`        | List invites  |
| POST   | `/api/tenant/invites/accept` | Accept invite |
| DELETE | `/api/tenant/invites/:id`    | Cancel invite |

#### Membership Management

| Method | Endpoint                                   | Description           |
| ------ | ------------------------------------------ | --------------------- |
| GET    | `/api/tenant/members`                      | List tenant members   |
| PATCH  | `/api/tenant/members/:id`                  | Update member roles   |
| DELETE | `/api/tenant/members/:id`                  | Remove member         |
| GET    | `/api/tenant/projects/:id/members`         | List project members  |
| POST   | `/api/tenant/projects/:id/members`         | Add project member    |
| DELETE | `/api/tenant/projects/:id/members/:userId` | Remove project member |

#### Tenant Management

| Method | Endpoint               | Description                |
| ------ | ---------------------- | -------------------------- |
| GET    | `/api/tenant/settings` | Get tenant settings        |
| PATCH  | `/api/tenant/settings` | Update tenant settings     |
| DELETE | `/api/tenant`          | Delete tenant (owner only) |

---

## 🔧 Environment Variables

Add to your `.env` file:

```env
# Tenant JWT (separate from dynamic route JWT)
TENANT_JWT_SECRET=your-tenant-secret-key-here

# Existing (for dynamic routes)
JWT_SECRET=your-existing-jwt-secret
```

---

## 🎯 Key Differences: Tenant Auth vs Dynamic Auth

| Feature         | Tenant Auth                         | Dynamic Auth               |
| --------------- | ----------------------------------- | -------------------------- |
| **Purpose**     | Manage platform (containers, pages) | Access data in containers  |
| **Users**       | Developers, admins, team members    | End users, customers       |
| **Routes**      | `/api/v1/container`, `/api/v1/page` | `/api/v1/dynamic/*`        |
| **Token**       | TENANT_JWT_SECRET                   | JWT_SECRET                 |
| **Middleware**  | `middlewares.TenantAuthenticate`    | `middlewares.Authenticate` |
| **Collections** | `users`, `tenant_memberships`       | Dynamic containers         |

---

## 📝 Example: Complete Developer Onboarding

```bash
# 1. Owner creates account & tenant
curl -X POST http://localhost:8080/api/tenant/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "owner@acme.com",
    "password": "Pass123!",
    "name": "Owner",
    "tenantName": "Acme Corp",
    "tenantSlug": "acme"
  }'
# Save accessToken as OWNER_TOKEN

# 2. Owner creates project (TODO)
curl -X POST http://localhost:8080/api/tenant/projects \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Mobile App",
    "slug": "mobile"
  }'
# Save project.id as PROJECT_ID

# 3. Owner invites developer (TODO)
curl -X POST http://localhost:8080/api/tenant/invites \
  -H "Authorization: Bearer $OWNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "dev@example.com",
    "scope": "project",
    "projectId": "'$PROJECT_ID'",
    "rolesToAssign": ["project_developer"]
  }'
# Save invite.token as INVITE_TOKEN

# 4. Developer registers
curl -X POST http://localhost:8080/api/tenant/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "dev@example.com",
    "password": "DevPass123!",
    "name": "Developer"
  }'
# Save accessToken as DEV_TOKEN

# 5. Developer accepts invite (TODO)
curl -X POST http://localhost:8080/api/tenant/invites/accept \
  -H "Authorization: Bearer $DEV_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "inviteToken": "'$INVITE_TOKEN'"
  }'

# 6. Developer switches to project
curl -X POST http://localhost:8080/api/tenant/auth/switch-project \
  -H "Authorization: Bearer $DEV_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "projectId": "'$PROJECT_ID'"
  }'
# Save new accessToken as PROJECT_TOKEN

# 7. Developer creates container
curl -X POST http://localhost:8080/api/v1/container \
  -H "Authorization: Bearer $PROJECT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "schemaName": "products",
    "fields": [...]
  }'
```

---

## 🚀 Next Steps

1. ✅ **Authentication System** - DONE
2. ⏳ **Project Management APIs** - Need to implement
3. ⏳ **Invite System** - Need to implement
4. ⏳ **Membership Management** - Need to implement
5. ⏳ **Tenant Settings** - Need to implement
6. ⏳ **Update Container Model** - Add tenantId/projectId filtering
7. ⏳ **Frontend Integration** - Build UI for all flows

---

## 💡 Questions to Consider

1. **Can a user create multiple tenants?**

   - Currently: YES (each register creates a tenant)
   - Suggestion: Allow users to create additional tenants after registration

2. **What happens when a user leaves a project?**

   - Delete ProjectMembership → User loses access
   - Keep TenantMembership if they're in other projects

3. **Can project members invite others?**

   - Currently: No role check on invite creation
   - Suggestion: Only `project_admin` can invite

4. **Should containers be tenant-scoped or project-scoped?**
   - **Answer:** Project-scoped (more isolation)
   - Each container should have `projectId` field

---

## 📚 Related Files

- Models: `/models/tenantModel.go`
- Auth Controller: `/controllers/tenantAuthController.go`
- Middleware: `/middlewares/tenantAuthenticate.go`
- Routes: `/routes/tenantAuthRoutes.go`
- JWT Utils: `/utils/tenantJwt.go`
- Container Routes: `/routes/containerRoutes.go` (updated)
- Page Routes: `/routes/pageRoutes.go` (updated)

---

**Last Updated:** December 18, 2025
