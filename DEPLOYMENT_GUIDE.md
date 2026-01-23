# Digital Ocean Deployment Guide

This guide will help you deploy your Go application to Digital Ocean using GitHub Actions.

## Prerequisites

1. **Digital Ocean Droplet** (Ubuntu 20.04 or later recommended)
2. **GitHub Repository** with your code
3. **Docker and Docker Compose** installed on your droplet

## Step 1: Prepare Your Digital Ocean Droplet

### 1.1 Create a Droplet

- Go to Digital Ocean dashboard
- Create a new Ubuntu 22.04 droplet
- Choose a plan (Basic $6/month should work for small apps)
- Add your SSH key during creation

### 1.2 SSH into Your Droplet

```bash
ssh root@your_droplet_ip
```

### 1.3 Install Docker and Docker Compose

```bash
# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh

# Start Docker service
systemctl start docker
systemctl enable docker

# Install Docker Compose
apt install docker-compose-plugin -y

# Verify installations
docker --version
docker compose version
```

### 1.4 Create a Deploy User (Optional but Recommended)

```bash
# Create user
adduser deploy

# Add to docker group
usermod -aG docker deploy

# Switch to deploy user
su - deploy
```

## Step 2: Push Your Code to GitHub

### 2.1 Create a GitHub Repository (if you haven't already)

1. Go to [GitHub](https://github.com) and sign in
2. Click the "+" icon in the top right → "New repository"
3. Name it (e.g., `autotable-go`)
4. Choose public or private
5. Click "Create repository"

### 2.2 Push Your Code

```bash
# In your project directory
git init
git add .
git commit -m "Initial commit - setup deployment"

# Add your GitHub repository as remote
git remote add origin https://github.com/YOUR_USERNAME/YOUR_REPO_NAME.git

# Push to main branch
git branch -M main
git push -u origin main
```

## Step 3: Configure GitHub Secrets

This is **CRITICAL** - the deployment won't work without these secrets!

### 3.1 Navigate to Secrets Settings

1. Go to your GitHub repository page
2. Click **Settings** (top menu)
3. In the left sidebar, click **Secrets and variables** → **Actions**
4. Click **New repository secret** button

### 3.2 Add Each Secret One by One

For each secret below, click "New repository secret", enter the **Name** exactly as shown, paste the **Value**, and click "Add secret".

### Required Secrets:

| Secret Name            | Description                       | Example                                           |
| ---------------------- | --------------------------------- | ------------------------------------------------- |
| `DROPLET_IP`           | Your Digital Ocean droplet IP     | `164.92.123.45`                                   |
| `SSH_USER`             | SSH user on droplet               | `root` or `deploy`                                |
| `SSH_PRIVATE_KEY`      | Private SSH key to access droplet | (your private key content)                        |
| `PORT_NUMBER`          | Application port                  | `4000`                                            |
| `MONGO_URI_BASE`       | MongoDB connection base           | `mongodb+srv://user:pass@cluster.mongodb.net/`    |
| `COLLECTION_NAME`      | MongoDB database name             | `autotable_prod`                                  |
| `MONGO_URI_SUFFIX`     | MongoDB URI suffix                | `?retryWrites=true&w=majority`                    |
| `JWT_SECRET`           | JWT secret key                    | Generate with: `openssl rand -base64 32`          |
| `CONTAINER_JWT_SECRET` | Container JWT secret              | Generate with: `openssl rand -base64 32`          |
| `TENANT_JWT_SECRET`    | Tenant JWT secret                 | Generate with: `openssl rand -base64 32`          |
| `CLOUD_NAME`           | Cloudinary cloud name             | Your Cloudinary cloud name                        |
| `CLOUD_API_KEY`        | Cloudinary API key                | Your Cloudinary API key                           |
| `CLOUD_API_SECRET`     | Cloudinary API secret             | Your Cloudinary API secret                        |
| `GOOGLE_CLIENT_ID`     | Google OAuth client ID            | `xxx.apps.googleusercontent.com`                  |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret        | Your Google client secret                         |
| `GOOGLE_REDIRECT_URL`  | Google OAuth redirect URL         | `https://yourdomain.com/auth/google/callback`     |
| `BACKEND_BASE_URL`     | Backend base URL                  | `https://yourdomain.com` or `http://your_ip:4000` |
| `FRONTEND_URL`         | Frontend URL                      | `https://yourfrontend.com`                        |

### How to Get Your SSH Private Key:

On your local machine:

```bash
# If you don't have an SSH key, generate one
ssh-keygen -t ed25519 -C "github-actions"

# Display your private key
cat ~/.ssh/id_ed25519
```

Copy the entire output (including `-----BEGIN OPENSSH PRIVATE KEY-----` and `-----END OPENSSH PRIVATE KEY-----`)

Then add your public key to the droplet:

```bash
# On your local machine
cat ~/.ssh/id_ed25519.pub

# SSH into droplet and add the public key
ssh root@your_droplet_ip
mkdir -p ~/.ssh
echo "your-public-key-here" >> ~/.ssh/authorized_keys
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys
```

### 3.3 How to Generate Secret Values

**For JWT Secrets** (run these 3 times for 3 different secrets):

```bash
openssl rand -base64 32
```

**MongoDB Connection String:**

- Go to [MongoDB Atlas](https://cloud.mongodb.com/)
- Click "Connect" on your cluster
- Choose "Connect your application"
- Copy the connection string
- Split it into parts:
  - `MONGO_URI_BASE`: `mongodb+srv://username:password@cluster.mongodb.net/`
  - `COLLECTION_NAME`: Your database name (e.g., `autotable_prod`)
  - `MONGO_URI_SUFFIX`: `?retryWrites=true&w=majority`

**Cloudinary Keys:**

- Go to [Cloudinary Dashboard](https://cloudinary.com/console)
- Find your Cloud Name, API Key, and API Secret

**Google OAuth:**

- Go to [Google Cloud Console](https://console.cloud.google.com/)
- Create OAuth 2.0 credentials
- Copy Client ID and Client Secret

### 3.4 Verify All Secrets Are Added

After adding all secrets, you should see 17 secrets in total:

- ✅ DROPLET_IP
- ✅ SSH_USER
- ✅ SSH_PRIVATE_KEY
- ✅ PORT_NUMBER
- ✅ MONGO_URI_BASE
- ✅ COLLECTION_NAME
- ✅ MONGO_URI_SUFFIX
- ✅ JWT_SECRET
- ✅ CONTAINER_JWT_SECRET
- ✅ TENANT_JWT_SECRET
- ✅ CLOUD_NAME
- ✅ CLOUD_API_KEY
- ✅ CLOUD_API_SECRET
- ✅ GOOGLE_CLIENT_ID
- ✅ GOOGLE_CLIENT_SECRET
- ✅ GOOGLE_REDIRECT_URL
- ✅ BACKEND_BASE_URL
- ✅ FRONTEND_URL

## Step 4: Configure Domain (Optional)

If you have a domain:

1. Add an A record pointing to your droplet IP
2. Install Nginx as reverse proxy:

```bash
# Install Nginx
apt install nginx -y

# Create Nginx configuration
nano /etc/nginx/sites-available/autotable
```

Add this configuration:

```nginx
server {
    listen 80;
    server_name yourdomain.com www.yourdomain.com;

    location / {
        proxy_pass http://localhost:4000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable the site:

```bash
ln -s /etc/nginx/sites-available/autotable /etc/nginx/sites-enabled/
nginx -t
systemctl restart nginx
```

Install SSL with Let's Encrypt:

```bash
apt install certbot python3-certbot-nginx -y
certbot --nginx -d yourdomain.com -d www.yourdomain.com
```

## Step 4: Add Health Check Endpoint

Your application should have a health check endpoint. Add this to your [routes](routes/) or create a new route:

```go
// In your main.go or a routes file
app.Get("/health", func(c *fiber.Ctx) error {
    return c.JSON(fiber.Map{
        "status": "healthy",
        "timestamp": time.Now(),
    })
})
```

## Step 5: Deploy

### 5.1 Trigger Your First Deployment

Once all secrets are configured, deploy by pushing to the main branch:

```bash
# Make sure all changes are committed
git add .
git commit -m "Setup deployment configuration"
git push origin main
```

### 5.2 Monitor the Deployment

1. Go to your GitHub repository
2. Click on the **Actions** tab (top menu)
3. You'll see a workflow run called "Deploy to Digital Ocean"
4. Click on it to see the live deployment logs
5. Wait for all steps to complete (usually 3-5 minutes)

### 5.3 What the GitHub Action Does Automatically:

1. ✅ Checks out your code
2. ✅ Sets up Go environment
3. ✅ Runs your tests (`go test ./...`)
4. ✅ Creates .env file with all your secrets
5. ✅ Connects to your droplet via SSH
6. ✅ Copies all files to the server using rsync
7. ✅ Builds Docker images
8. ✅ Starts containers with docker-compose
9. ✅ Runs health check to verify deployment
10. ✅ Cleans up old images

### 5.4 If Deployment Succeeds

You'll see a green checkmark ✅ next to the workflow. Your app is now live at:

- `http://YOUR_DROPLET_IP:4000`

Test it:

```bash
curl http://YOUR_DROPLET_IP:4000/health
```

### 5.5 If Deployment Fails

1. Click on the failed workflow in GitHub Actions
2. Expand the failed step to see error details
3. Common issues:
   - **SSH connection failed**: Check `SSH_PRIVATE_KEY` and `DROPLET_IP`
   - **Secret missing**: Verify all 17 secrets are added
   - **Tests failed**: Fix failing tests before deploying
   - **Docker build failed**: Check Dockerfile syntax
   - **MongoDB connection**: Add droplet IP to MongoDB Atlas whitelist

### 5.6 Future Deployments

Every time you push to `main` branch, it will automatically deploy:

```bash
# Make changes to your code
git add .
git commit -m "Add new feature"
git push origin main  # This triggers deployment automatically
```

You can also manually trigger deployment:

1. Go to GitHub → Actions tab
2. Click "Deploy to Digital Ocean"
3. Click "Run workflow" → "Run workflow"

## Step 6: Monitor Your Application

3. Copy files
4. Build Docker images
5. Start services with docker-compose
6. Run health check

### Manual Deployment from Droplet:

You can also manually deploy by SSH'ing into your droplet:

```bash
ssh root@your_droplet_ip
cd ~/autotable
git pull origin main  # if you cloned the repo
docker-compose down
docker-compose up -d --build
docker-compose logs -f  # View logs
```

## Step 6: Monitor Your Application

### View logs:

```bash
ssh root@your_droplet_ip
cd ~/autotable
docker-compose logs -f app      # Application logs
docker-compose logs -f redis    # Redis logs
```

### Check container status:

```bash
docker-compose ps
```

### Restart services:

```bash
docker-compose restart
```

### Stop services:

```bash
docker-compose down
```

## Troubleshooting

### Container won't start:

```bash
docker-compose logs app
```

### MongoDB connection issues:

- Check your MongoDB Atlas IP whitelist (add your droplet IP or allow all: `0.0.0.0/0`)
- Verify `MONGO_URI_BASE`, `COLLECTION_NAME`, and `MONGO_URI_SUFFIX` are correct

### Port already in use:

```bash
# Check what's using port 4000
lsof -i :4000
# Kill the process
kill -9 <PID>
```

### GitHub Action fails:

- Check SSH connection: `ssh root@your_droplet_ip`
- Verify all GitHub secrets are set correctly
- Check workflow logs in GitHub Actions tab

## Security Best Practices

1. **Use a non-root user** for deployments
2. **Enable UFW firewall**:
   ```bash
   ufw allow 22/tcp      # SSH
   ufw allow 80/tcp      # HTTP
   ufw allow 443/tcp     # HTTPS
   ufw allow 4000/tcp    # Your app (if not using Nginx)
   ufw enable
   ```
3. **Keep system updated**: `apt update && apt upgrade -y`
4. **Use environment variables** for all secrets
5. **Enable automatic security updates**
6. **Use strong passwords** and SSH keys only
7. **Set up monitoring** (Digital Ocean Monitoring, or use services like DataDog)

## Cost Optimization

- Basic droplet: $6/month
- MongoDB Atlas: Free tier (512MB)
- Cloudinary: Free tier
- Total: ~$6/month for small applications

## Next Steps

1. Set up automated backups for your droplet
2. Configure monitoring and alerts
3. Set up staging environment (optional)
4. Implement CI/CD for automated testing
5. Add log aggregation (e.g., Papertrail, Logtail)
6. Set up database backups

## Support

If you encounter issues:

1. Check the logs: `docker-compose logs -f`
2. Verify all environment variables are set
3. Check GitHub Actions workflow logs
4. Ensure MongoDB Atlas allows connections from your droplet IP
