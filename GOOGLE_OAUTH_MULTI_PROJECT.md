# Google OAuth for Multi-Project Authentication

## Overview

The Google OAuth implementation now supports **project-specific authentication**, ensuring that users authenticating via Google receive tokens scoped to the specific project they're logging into. This prevents users from accessing other projects using the same Google account token.

## How It Works

### 1. **GoogleLogin Flow** (`/api/:tenantSlug/:projectSlug/auth/google/login`)

When a user initiates Google login:

1. **Context Extraction**: The system extracts `tenantID`, `projectID`, `tenantSlug`, and `projectSlug` from the URL path
2. **State Generation**: Creates a unique CSRF protection state token (UUID)
3. **Context Storage**: Stores the tenant/project context in Redis with the state as the key:
   ```json
   {
     "tenantID": "507f1f77bcf86cd799439011",
     "projectID": "507f191e810c19729de860ea",
     "tenantSlug": "my-tenant",
     "projectSlug": "my-project"
   }
   ```
4. **OAuth Redirect**: Redirects user to Google OAuth consent screen with the state parameter

### 2. **GoogleCallback Flow** (`/api/:tenantSlug/:projectSlug/auth/google/callback`)

When Google redirects back after authentication:

1. **State Validation**: Validates the state parameter and retrieves it from Redis
2. **Context Retrieval**: Extracts the stored tenant/project context from Redis
3. **One-Time Use**: Deletes the state from Redis (prevents replay attacks)
4. **Token Exchange**: Exchanges authorization code for Google access token
5. **User Info**: Fetches user information from Google
6. **Project-Specific User**: Creates or finds user in the **project-specific** user collection
7. **Token Generation**: Generates JWT tokens scoped to the **specific tenant and project**

## Key Security Features

### Project Isolation

- Each project maintains its own user collection: `{tenantID}_{projectID}_users`
- Tokens are scoped with both `tenantID` and `projectID` claims
- A user logging in to Project A cannot use that token to access Project B

### CSRF Protection

- State parameter is a cryptographically secure UUID
- State is stored in Redis with 5-minute expiration
- State is validated and deleted on first use (one-time use)

### Context Integrity

- Tenant and project context are captured at login initiation
- Context is securely stored in Redis (not in client-side cookies)
- Context is restored exactly as captured during callback

## URL Structure

For Google OAuth to work with multi-project support, use this URL pattern:

```
Login URL:
https://yourdomain.com/api/:tenantSlug/:projectSlug/auth/google/login

Callback URL (configured in Google Cloud Console):
https://yourdomain.com/api/:tenantSlug/:projectSlug/auth/google/callback
```

### Example URLs

**Tenant: "acme", Project: "web-app"**

```
Login: https://api.example.com/api/acme/web-app/auth/google/login
Callback: https://api.example.com/api/acme/web-app/auth/google/callback
```

**Tenant: "acme", Project: "mobile-app"**

```
Login: https://api.example.com/api/acme/mobile-app/auth/google/login
Callback: https://api.example.com/api/acme/mobile-app/auth/google/callback
```

## Google Cloud Console Setup

### 1. Configure OAuth Consent Screen

- Go to Google Cloud Console → APIs & Services → OAuth consent screen
- Configure your app name, logo, and authorized domains

### 2. Create OAuth 2.0 Credentials

- Go to Credentials → Create Credentials → OAuth 2.0 Client ID
- Application type: Web application
- Add authorized redirect URIs:

**Option 1: Wildcard Support (if available)**

```
https://yourdomain.com/api/*/*/auth/google/callback
```

**Option 2: Specific Projects**
Add each tenant/project combination:

```
https://yourdomain.com/api/tenant1/project1/auth/google/callback
https://yourdomain.com/api/tenant1/project2/auth/google/callback
https://yourdomain.com/api/tenant2/project1/auth/google/callback
```

**Option 3: Dynamic Callback (Recommended)**
Use a single callback endpoint that doesn't require tenant/project in path:

```
https://yourdomain.com/api/oauth/google/callback
```

Then modify your routes to handle this centralized callback.

### 3. Environment Variables

Add to your `.env` file:

```env
GOOGLE_CLIENT_ID=your_client_id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your_client_secret
GOOGLE_REDIRECT_URL=https://yourdomain.com/api/oauth/google/callback
```

## Token Response

After successful authentication, the callback returns:

```json
{
  "status": 200,
  "message": "Google login successful",
  "data": {
    "accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user": {
      "_id": "507f1f77bcf86cd799439011",
      "email": "user@example.com",
      "name": "John Doe",
      "picture": "https://lh3.googleusercontent.com/...",
      "role": "user"
    }
  }
}
```

The JWT tokens contain claims:

```json
{
  "userId": "507f1f77bcf86cd799439011",
  "role": "user",
  "tenantId": "507f1f77bcf86cd799439011",
  "projectId": "507f191e810c19729de860ea",
  "tenantSlug": "acme",
  "projectSlug": "web-app",
  "exp": 1234567890
}
```

## Frontend Integration

### Example: React/Next.js

```javascript
// Initiate Google login
const handleGoogleLogin = () => {
  const tenantSlug = "acme";
  const projectSlug = "web-app";

  // Redirect to Google OAuth
  window.location.href = `https://api.example.com/api/${tenantSlug}/${projectSlug}/auth/google/login`;
};

// Handle callback (in your callback page)
useEffect(() => {
  const urlParams = new URLSearchParams(window.location.search);
  const error = urlParams.get("error");

  if (error) {
    console.error("OAuth error:", error);
    return;
  }

  // Backend will return tokens in response
  // Store them in localStorage/cookies
}, []);
```

### Example: Button Component

```jsx
<button onClick={handleGoogleLogin}>
  <img src="/google-icon.svg" alt="Google" />
  Sign in with Google
</button>
```

## Testing

### 1. Test Project Isolation

```bash
# User logs into Project A
curl -L "https://api.example.com/api/tenant1/projectA/auth/google/login"
# Get token_A

# Try to use token_A to access Project B
curl -H "Authorization: Bearer token_A" \
  "https://api.example.com/api/tenant1/projectB/data/users"
# Should return 403 Forbidden
```

### 2. Test State Validation

```bash
# Try to reuse a state token
# Should fail with "Invalid or expired OAuth state"
```

## Troubleshooting

### Error: "Failed to get project context"

- **Cause**: URL doesn't contain valid `tenantSlug` and `projectSlug`
- **Solution**: Ensure URL follows pattern `/api/:tenantSlug/:projectSlug/auth/google/login`

### Error: "Invalid or expired OAuth state"

- **Cause**: State token expired (>5 minutes) or already used
- **Solution**: Restart the OAuth flow from the login page

### Error: "Authentication container not configured"

- **Cause**: Project doesn't have a container with `isAuthContainer: true`
- **Solution**: Configure an auth container for the project with email field

### Error: "redirect_uri_mismatch"

- **Cause**: Google redirect URI doesn't match configured URI in Google Console
- **Solution**: Add all required redirect URIs to Google Cloud Console

## Migration from Single-Project OAuth

If you had a previous Google OAuth implementation without project context:

### Before

```javascript
// Old: No project context
window.location.href = `https://api.example.com/auth/google/login`;
```

### After

```javascript
// New: With project context
const { tenantSlug, projectSlug } = getProjectContext();
window.location.href = `https://api.example.com/api/${tenantSlug}/${projectSlug}/auth/google/login`;
```

## Benefits

✅ **Project Isolation**: Users cannot access other projects with same Google account
✅ **Audit Trail**: Clear audit logs per project for Google logins
✅ **Scalability**: Support unlimited tenants and projects
✅ **Security**: CSRF protection with one-time state tokens
✅ **Flexibility**: Each project can have different user schemas
✅ **Compliance**: Tenant data isolation for regulatory requirements

## Related Documentation

- [Multi-Tenancy Guide](./MULTI_TENANCY_GUIDE.md)
- [Google OAuth Setup](./GOOGLE_OAUTH_SETUP.md)
- [JWT Authentication](./JWT_AUTH.md)
