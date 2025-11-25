# OAuth State Parameter - Security Explanation

## Why NOT to Put State in Environment Variables

### The Problem
The `state` parameter in OAuth is designed to prevent **CSRF (Cross-Site Request Forgery) attacks**. 

### Requirements for Secure State
1. ✅ **Unique per request** - Different for each user's login attempt
2. ✅ **Unpredictable** - Cryptographically random
3. ✅ **Temporary** - Expires after a short time
4. ✅ **One-time use** - Deleted after validation

### Why .env is NOT Secure

If you put the state in `.env`:
- ❌ **Same for all users** - Every user would have the same state
- ❌ **Same for all requests** - An attacker could reuse it
- ❌ **Never expires** - Remains valid forever
- ❌ **Predictable** - Anyone with access to your code/env knows it

**This would make CSRF attacks EASIER, not harder!**

## Our Implementation (Secure)

### What We Do

```go
// 1. Generate unique random state (UUID v4)
state := uuid.New().String()
// Example: "f47ac10b-58cc-4372-a567-0e02b2c3d479"

// 2. Store in Redis with 5-minute expiration
redisClient.Set(ctx, "oauth:state:" + state, "valid", 5*time.Minute)

// 3. Send to Google as part of OAuth URL
url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)

// 4. When Google redirects back, validate the state
receivedState := c.Query("state")
storedValue := redisClient.Get(ctx, "oauth:state:" + receivedState)

// 5. Delete after use (one-time only)
redisClient.Del(ctx, "oauth:state:" + receivedState)
```

### Security Benefits

1. **Unique per request**: Each login attempt gets a new UUID
2. **Unpredictable**: UUID v4 is cryptographically random
3. **Temporary**: Expires after 5 minutes
4. **One-time use**: Deleted immediately after validation
5. **Stateless**: Works across multiple server instances (stored in Redis)

### Attack Prevention

**Without proper state (e.g., using .env):**
```
Attacker creates malicious link:
https://yourapp.com/auth/google/callback?code=ATTACKER_CODE&state=known-static-value

Victim clicks link → Attacker's Google account linked to victim's session
```

**With our implementation:**
```
Attacker creates malicious link:
https://yourapp.com/auth/google/callback?code=ATTACKER_CODE&state=random-uuid

Server checks Redis → State not found or expired → Request rejected ✅
```

## Alternative Approaches

If you don't want to use Redis, you could:

### 1. Session-based (requires session middleware)
```go
// Store in session
session.Set("oauth_state", state)

// Validate from session
if session.Get("oauth_state") != receivedState {
    return error
}
```

### 2. Signed JWT (stateless but more complex)
```go
// Create signed JWT with expiration
token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
    "exp": time.Now().Add(5 * time.Minute).Unix(),
})
state, _ := token.SignedString([]byte(secret))

// Validate JWT signature and expiration
token, err := jwt.Parse(receivedState, ...)
```

### 3. Encrypted cookie (stateless)
```go
// Encrypt state and store in cookie
encryptedState := encrypt(state, secret)
c.Cookie(&fiber.Cookie{
    Name:    "oauth_state",
    Value:   encryptedState,
    Expires: time.Now().Add(5 * time.Minute),
})
```

## Why We Chose Redis

✅ **Already in your stack** - You're using Redis for caching  
✅ **Simple and reliable** - Easy to implement and understand  
✅ **Automatic expiration** - Built-in TTL support  
✅ **Scalable** - Works across multiple server instances  
✅ **Fast** - In-memory storage for quick validation  

## Summary

**Question:** Is putting state in `.env` more secure?

**Answer:** **No, it's MUCH LESS secure!** 

The state must be:
- Random and unique per request
- Temporary (expires quickly)
- One-time use

Our implementation using **UUID + Redis** provides proper CSRF protection while being simple and maintainable.
