# Google OAuth Setup Guide

## Environment Variables

Add the following variables to your `.env` file:

```env
# Google OAuth Configuration
GOOGLE_CLIENT_ID=your-client-id-here.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-client-secret-here
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
```

**Note:** Replace `8080` with your actual `PORT_NUMBER` from your `.env` file.

## How to Get Google OAuth Credentials

### Step 1: Go to Google Cloud Console
Visit [Google Cloud Console](https://console.cloud.google.com/)

### Step 2: Create or Select a Project
- Click on the project dropdown at the top
- Create a new project or select an existing one

### Step 3: Enable Required APIs
- Go to "APIs & Services" → "Library"
- Search for and enable "Google+ API" or "People API"

### Step 4: Create OAuth 2.0 Credentials
1. Go to "APIs & Services" → "Credentials"
2. Click "Create Credentials" → "OAuth 2.0 Client ID"
3. If prompted, configure the OAuth consent screen:
   - Choose "External" for user type (or "Internal" if using Google Workspace)
   - Fill in app name, user support email, and developer contact
   - Add scopes: `userinfo.email` and `userinfo.profile`
   - Add test users if in testing mode

4. Choose "Web application" as the application type
5. Add authorized redirect URIs:
   - For local development: `http://localhost:8080/api/v1/auth/google/callback`
   - For production: `https://yourdomain.com/api/v1/auth/google/callback`

6. Click "Create"
7. Copy the **Client ID** and **Client Secret**

### Step 5: Update Your .env File
Paste the credentials into your `.env` file:

```env
GOOGLE_CLIENT_ID=123456789-abcdefghijklmnop.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-your_secret_here
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
```

## How It Works

### Authentication Flow

1. **User initiates login**: Navigate to `http://localhost:8080/api/v1/auth/google/login`
2. **Redirect to Google**: User is redirected to Google's consent screen
3. **User authorizes**: User logs in and grants permissions
4. **Callback**: Google redirects back to `/api/v1/auth/google/callback` with an authorization code
5. **Token exchange**: Server exchanges the code for access tokens
6. **User info retrieval**: Server fetches user profile from Google
7. **User creation/login**: 
   - If user exists (matched by email), log them in
   - If user doesn't exist, create a new user with Google profile data
8. **JWT generation**: Server generates JWT tokens using your existing auth system
9. **Response**: Returns access token, refresh token, and user data

### Requirements for Auth Container

Your auth container (the schema with `IsAuthContainer: true`) **must have**:
- An `email` field (type: "email" or name: "email")

The system will:
- Automatically find the auth container
- Match users by email
- Create new users with:
  - Email from Google
  - Name from Google (if available)
  - Picture URL from Google (if available)
  - Default role: "user"

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v1/auth/google/login` | GET | Initiates Google OAuth flow |
| `/api/v1/auth/google/callback` | GET | Handles OAuth callback (used by Google) |

### Response Format

Successful login returns:

```json
{
  "status": 200,
  "message": "Google login successful",
  "data": {
    "accessToken": "eyJhbGciOiJIUzI1NiIs...",
    "refreshToken": "eyJhbGciOiJIUzI1NiIs...",
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

## Testing

1. Ensure your server is running
2. Navigate to: `http://localhost:8080/api/v1/auth/google/login`
3. You should be redirected to Google's login page
4. After successful login, you'll be redirected back with tokens
5. Check your database to verify the user was created

## Production Deployment

For production:

1. Update `GOOGLE_REDIRECT_URL` to your production domain
2. Add the production URL to authorized redirect URIs in Google Cloud Console
3. Consider implementing CSRF protection (currently using a placeholder state)
4. Review and adjust the default role assignment logic
5. Consider adding email verification requirements

## Troubleshooting

### "Authorization code not found"
- Check that the callback URL matches exactly in Google Cloud Console
- Ensure the redirect URL in `.env` matches your server configuration

### "Auth container must have an email field"
- Add an email field to your auth container schema
- Field should have `type: "email"` or `name: "email"`

### "Failed to exchange authorization code"
- Verify your `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` are correct
- Check that the OAuth consent screen is properly configured
- Ensure you've added test users if the app is in testing mode
