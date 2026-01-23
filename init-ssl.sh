#!/bin/bash

# Script to initialize SSL certificates with Let's Encrypt

DOMAIN="api-production.autoapi.org"
EMAIL="your-email@example.com"  # Change this!

echo "=== SSL Setup for $DOMAIN ==="
echo ""
echo "IMPORTANT: Update EMAIL in this script before running!"
echo "Press Ctrl+C to cancel, or Enter to continue..."
read

# Create certbot directories
mkdir -p certbot/conf certbot/www

# Temporarily use init config (without SSL)
echo "Step 1: Using temporary nginx config..."
cp nginx-init.conf nginx.conf

# Restart nginx with init config
echo "Step 2: Restarting nginx..."
docker compose down
docker compose up -d nginx

# Wait for nginx to start
echo "Waiting for nginx to start..."
sleep 5

# Request certificate
echo "Step 3: Requesting SSL certificate from Let's Encrypt..."
docker compose run --rm certbot certonly --webroot \
    --webroot-path=/var/www/certbot \
    --email $EMAIL \
    --agree-tos \
    --no-eff-email \
    -d $DOMAIN

# Check if certificate was obtained
if [ -d "certbot/conf/live/$DOMAIN" ]; then
    echo "✅ Certificate obtained successfully!"
    
    # Restore SSL-enabled nginx config
    echo "Step 4: Restoring SSL nginx config..."
    git checkout nginx.conf
    
    # Restart nginx with SSL
    echo "Step 5: Restarting nginx with SSL..."
    docker compose down
    docker compose up -d
    
    echo ""
    echo "✅ SSL setup complete!"
    echo "Your API should now be available at: https://$DOMAIN"
else
    echo "❌ Failed to obtain certificate"
    echo "Please check the errors above and try again"
    exit 1
fi
