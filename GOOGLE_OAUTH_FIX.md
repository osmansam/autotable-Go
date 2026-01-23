# Google OAuth Setup - Quick Fix Guide

## Problem

Getting `redirect_uri_mismatch` error from Google OAuth.

## Root Cause

The redirect URL configured in your `.env` file doesn't match:

1. The URL registered in Google Cloud Console
2. The URL your backend is sending to Google

## Solution

### Step 1: Update .env file ✅ (Already Done)

```env
GOOGLE_REDIRECT_URL=http://localhost:3002/api/v1/auth/google/callback
BACKEND_BASE_URL=http://localhost:3002/api/v1
```

### Step 2: Update Google Cloud Console (DO THIS NOW)

1. Go to: https://console.cloud.google.com/apis/credentials
2. Select your project
3. Click on your OAuth 2.0 Client ID: `444141296200-7d36h2qarnbndnjbvthcirfc84juteji`
4. In **Authorized redirect URIs**, add these URLs:

```
http://localhost:3002/api/v1/auth/google/callback
http://localhost:3002/auth/google/callback
```

### Step 3: For Multi-Tenant Support (Optional for Production)

For each tenant/project combination you want to support, add:

```
http://localhost:3002/api/v1/{tenantSlug}/{projectSlug}/auth/google/callback
```

Example:

```
http://localhost:3002/api/v1/acme/web-app/auth/google/callback
http://localhost:3002/api/v1/acme/mobile-app/auth/google/callback
```

**Note:** Google doesn't support wildcard patterns in redirect URIs, so you need to add each combination manually.

### Step 4: Production URLs

For production, also add your production domain:

```
https://yourdomain.com/api/v1/auth/google/callback
https://yourdomain.com/api/v1/{tenant}/{project}/auth/google/callback
```

## How It Works Now

1. **User clicks "Sign in with Google"** on frontend

   - Frontend redirects to: `http://localhost:3002/api/v1/{tenant}/{project}/auth/google/login`

2. **Backend GoogleLogin handler**:

   - Extracts tenant/project from URL
   - Stores context in Redis with state token
   - Constructs dynamic redirect URL: `http://localhost:3002/api/v1/{tenant}/{project}/auth/google/callback`
   - Redirects user to Google with this callback URL

3. **Google redirects back to**:

   - The exact callback URL that was sent (with tenant/project)
   - Example: `http://localhost:3002/api/v1/acme/web-app/auth/google/callback?code=xxx&state=yyy`

4. **Backend GoogleCallback handler**:
   - Validates state and retrieves tenant/project context from Redis
   - Exchanges code for tokens with Google
   - Creates/finds user in project-specific collection
   - Generates project-scoped JWT tokens
   - Returns tokens to frontend

## Restart Required

After updating the `.env` file, restart your Go server:

```bash
# Stop current server (Ctrl+C)
go run main.go
```

## Testing

1. Make sure your backend is running on port 3002
2. Make sure your frontend is running (probably on a different port)
3. Try Google login again
4. Check the logs - you should see:
   ```
   [GoogleLogin] Dynamic redirect URL: http://localhost:3002/api/v1/{tenant}/{project}/auth/google/callback
   ```

## Troubleshooting

### Still getting redirect_uri_mismatch?

- Double-check the URLs in Google Cloud Console match exactly (including http:// vs https://)
- Wait a few minutes after updating Google Cloud Console (changes may take time to propagate)
- Clear your browser cache and cookies
- Check the logs to see what redirect URL is being sent to Google

### Logs to check:

```
[GoogleLogin] OAuth Config - ClientID: xxx, RedirectURL: http://localhost:3002/api/v1/...
[GoogleLogin] Redirecting to Google OAuth URL: https://accounts.google.com/o/oauth2/auth?...
```

The redirect URL in the logs should match one of the URLs in Google Cloud Console.

## Frontend Integration

Your frontend code looks correct. It constructs the login URL as:

```javascript
const googleLoginUrl = `${baseUrl}/${tenant}/${project}/auth/google/login`;
```

This will call your backend's GoogleLogin handler with the tenant/project context.

## Alternative: Centralized Callback URL

If you don't want to add multiple redirect URIs to Google Cloud Console, you can use a single centralized callback URL and handle routing in your backend. The current implementation already supports this through the state parameter in Redis.

Just add this single URL to Google Cloud Console:

```
http://localhost:3002/api/v1/auth/google/callback
```

And modify the backend to always use this URL instead of constructing dynamic ones. The tenant/project context is still preserved in the Redis state.
