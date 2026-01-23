# GitHub Setup Quick Guide

This is a quick reference for setting up GitHub for automated deployment to Digital Ocean.

## 🔑 Your SSH Config

You're using SSH alias `autotable`. Your config shows:

- **IP Address**: `146.190.115.74`
- **Username**: `deploy`
- **SSH Key**: `/Users/osmansamilerdogan/.ssh/autotable`

You can connect with: `ssh autotable`

Use these values when setting up GitHub secrets below! ⬇️

## 1️⃣ Create GitHub Repository

1. Go to https://github.com and sign in
2. Click the "+" icon (top right) → "New repository"
3. Fill in:
   - **Repository name**: `autotable-go` (or your preferred name)
   - **Visibility**: Public or Private (your choice)
4. Click **"Create repository"**
5. **IMPORTANT**: Copy the repository URL shown (e.g., `https://github.com/yourusername/autotable-go.git`)

## 2️⃣ Push Your Code to GitHub

Open terminal in your project directory and run:

```bash
# Initialize git (if not already done)
git init

# Add all files
git add .

# Make first commit
git commit -m "Initial commit - setup deployment"

# Add GitHub as remote (replace with YOUR repository URL)
git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO_NAME.git

# Rename branch to main
git branch -M main

# Push to GitHub
git push -u origin main
```

## 3️⃣ Add GitHub Secrets (CRITICAL STEP!)

### How to Access Secrets Settings:

1. Go to your repository on GitHub
2. Click **"Settings"** tab (top menu, far right)
3. Look at the left sidebar
4. Click **"Secrets and variables"** → **"Actions"**
5. Click the green **"New repository secret"** button

### Add These 17 Secrets:

For each secret:

- Click "New repository secret"
- Enter the **Name** exactly as shown below
- Paste the **Value**
- Click "Add secret"

#### Server Connection Secrets:

| Name              | Value                   | How to Get It                                                 |
| ----------------- | ----------------------- | ------------------------------------------------------------- |
| `DROPLET_IP`      | `146.190.115.74`        | ✅ Your droplet IP (from your SSH config above)               |
| `SSH_USER`        | `deploy`                | ✅ Your SSH user (from your SSH config above)                 |
| `SSH_PRIVATE_KEY` | `-----BEGIN OPENSSH...` | Run `cat /Users/osmansamilerdogan/.ssh/autotable` to get this |

#### Application Configuration:

| Name          | Value        | How to Get It                  |
| ------------- | ------------ | ------------------------------ |
| `PORT_NUMBER` | `4000`       | Your app port (keep as `4000`) |
| `NODE_ENV`    | `production` | Environment name               |

#### Database Secrets:

| Name               | Value                                          | How to Get It                        |
| ------------------ | ---------------------------------------------- | ------------------------------------ |
| `MONGO_URI_BASE`   | `mongodb+srv://user:pass@cluster.mongodb.net/` | From MongoDB Atlas connection string |
| `COLLECTION_NAME`  | `autotable_prod`                               | Your database name                   |
| `MONGO_URI_SUFFIX` | `?retryWrites=true&w=majority`                 | Connection options                   |

#### Security Secrets (Generate These!):

| Name                   | How to Generate                |
| ---------------------- | ------------------------------ |
| `JWT_SECRET`           | Run: `openssl rand -base64 32` |
| `CONTAINER_JWT_SECRET` | Run: `openssl rand -base64 32` |
| `TENANT_JWT_SECRET`    | Run: `openssl rand -base64 32` |

#### Cloudinary Secrets:

| Name               | Where to Find        |
| ------------------ | -------------------- |
| `CLOUD_NAME`       | Cloudinary Dashboard |
| `CLOUD_API_KEY`    | Cloudinary Dashboard |
| `CLOUD_API_SECRET` | Cloudinary Dashboard |

Go to: https://cloudinary.com/console

#### Google OAuth Secrets:

| Name                   | Where to Find                                              |
| ---------------------- | ---------------------------------------------------------- |
| `GOOGLE_CLIENT_ID`     | Google Cloud Console → APIs & Services → Credentials       |
| `GOOGLE_CLIENT_SECRET` | Google Cloud Console → APIs & Services → Credentials       |
| `GOOGLE_REDIRECT_URL`  | Set to: `http://YOUR_DROPLET_IP:4000/auth/google/callback` |

#### URLs:

| Name               | Value Example                                             |
| ------------------ | --------------------------------------------------------- |
| `BACKEND_BASE_URL` | `http://YOUR_DROPLET_IP:4000` or `https://yourdomain.com` |
| `FRONTEND_URL`     | `http://localhost:3000` or your frontend URL              |

### ✅ Verification Checklist

After adding all secrets, verify you have exactly **17 secrets**:

- [ ] DROPLET_IP
- [ ] SSH_USER
- [ ] SSH_PRIVATE_KEY
- [ ] PORT_NUMBER
- [ ] MONGO_URI_BASE
- [ ] COLLECTION_NAME
- [ ] MONGO_URI_SUFFIX
- [ ] JWT_SECRET
- [ ] CONTAINER_JWT_SECRET
- [ ] TENANT_JWT_SECRET
- [ ] CLOUD_NAME
- [ ] CLOUD_API_KEY
- [ ] CLOUD_API_SECRET
- [ ] GOOGLE_CLIENT_ID
- [ ] GOOGLE_CLIENT_SECRET
- [ ] GOOGLE_REDIRECT_URL
- [ ] BACKEND_BASE_URL
- [ ] FRONTEND_URL

## 4️⃣ Deploy!

### First Deployment:

```bash
# Make sure all changes are committed
git add .
git commit -m "Ready for deployment"
git push origin main
```

### Watch the Deployment:

1. Go to your GitHub repository
2. Click the **"Actions"** tab
3. You'll see a workflow running: **"Deploy to Digital Ocean"**
4. Click on it to see real-time logs
5. Wait 3-5 minutes for completion

### Success! ✅

When you see all green checkmarks:

- Your app is live at `http://YOUR_DROPLET_IP:4000`
- Test: `curl http://YOUR_DROPLET_IP:4000/health`

### If It Fails ❌

1. Click on the failed workflow
2. Expand the red X step to see error
3. Common fixes:
   - Missing secret → Add it in Settings → Secrets
   - Wrong SSH key → Regenerate and re-add
   - MongoDB connection → Whitelist your droplet IP in MongoDB Atlas

## 5️⃣ Future Deployments

Every push to `main` automatically deploys:

```bash
# Make your code changes
git add .
git commit -m "Your changes"
git push origin main  # 🚀 Auto-deploys!
```

### Manual Deployment:

1. Go to GitHub → **Actions** tab
2. Click **"Deploy to Digital Ocean"** workflow
3. Click **"Run workflow"** dropdown
4. Click **"Run workflow"** button

## 🆘 Help

### Can't Find Settings?

Make sure you're logged into GitHub and viewing YOUR repository (not someone else's fork).

### SSH Key Issues?

```bash
# View your SSH config
cat ~/.ssh/config | grep -A 5 "Host autotable"

# Copy your private key (this goes in GitHub secret SSH_PRIVATE_KEY)
cat /Users/osmansamilerdogan/.ssh/autotable

# Test SSH connection to your server
ssh autotable

# Or with full command
ssh deploy@146.190.115.74

# Verify you can run commands on the server
ssh autotable "docker --version"
```

### Secret Value Has Quotes?

Don't include the quotes when pasting into GitHub secrets. Just paste the raw value.

### MongoDB Connection String?

Full string looks like: `mongodb+srv://username:password@cluster.mongodb.net/dbname?retryWrites=true&w=majority`

Split it:

- **MONGO_URI_BASE**: `mongodb+srv://username:password@cluster.mongodb.net/`
- **COLLECTION_NAME**: `dbname`
- **MONGO_URI_SUFFIX**: `?retryWrites=true&w=majority`

---

**Next**: See [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) for complete deployment documentation.
