# Deployment Guide

This guide explains how to deploy the Budget application on a Linux server (e.g., Ubuntu).
The application consists of two parts:

1. **Backend**: A Go server (binary) that manages the database and handles logic.
2. **Frontend**: A static HTML/JS Progressive Web App (PWA).

## Requirements

- A generic Linux server (e.g. Ubuntu or Debian).
- Root/Sudo access.
- A domain name (referenced as `your-domain.com` in this guide).

> [!IMPORTANT]
> **Same Hostname Requirement**
> The Frontend (`budget.html`) relies on `window.location.hostname` to find the API.
> Therefore, **budget.html MUST be served from the same server (hostname) as the API**, or you must configure a reverse proxy.

---

## Part 1: Backend Setup

### 1. Build the Binary

Cross-compile the Go application for your Linux server. Run this on your development machine:

```bash
GOOS=linux GOARCH=amd64 go build -o budget main.go
```

### 2. Copy Files to Server

Create a directory on your server and copy the necessary files.

```bash
# Create directory
ssh user@your-domain.com "sudo mkdir -p /opt/budget"

# Copy binary and configuration templates
scp budget users.example user@your-domain.com:/opt/budget/
```

### 3. Configure Users

On the server, you must create a `users` file. This file acts as an allowlist for who can access the budget.

1. Copy the example file:
   
   ```bash
   cd /opt/budget
   sudo cp users.example users
   ```

2. Edit the file:
   
   ```bash
   sudo nano users
   ```

3. Add one valid user ID per line (Case-sensitive, uppercase recommended for simplicity):
   
   ```text
   PAUL
   MARIA
   ```

### 4. Create Systemd Service

Set up the backend to run automatically in the background.

Create `/etc/systemd/system/budget.service`:

```ini
[Unit]
Description=Budget App Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/budget
ExecStart=/opt/budget/budget
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable budget
sudo systemctl start budget
```

Verify it's running:

```bash
sudo systemctl status budget
```

The server is now listening on port **8910** (HTTP).

---

## Part 2: Frontend Setup

You need to serve the content of the `budget` folder (containing `budget.html`, `sw.js`, `manifest.json`, etc.) so it is accessible via a browser.

### Option A: Using Nginx (Recommended)

This method serves the static files on port 80/443 and proxies API requests to the Go server.

1. Install Nginx: `sudo apt install nginx`
2. Copy your `budget` frontend folder to `/var/www/budget`.
3. Configure Nginx (e.g., `/etc/nginx/sites-available/budget`):

```nginx
server {
    listen 80;
    server_name your-domain.com;

    root /var/www/budget;
    index budget.html;

    location / {
        try_files $uri $uri/ =404;
    }

    # Proxy API requests to the Go backend
    # Note: If your app uses port 8910 directly in the JS, you might not
    # strictly need this proxy block if you open port 8910 in the firewall.
    # However, serving everything on port 80/443 is cleaner.
}
```

### Option B: Using Apache

If you prefer Apache.

1. Install Apache: `sudo apt install apache2`
2. Copy your `budget` frontend folder to `/var/www/html/budget`.
3. Ensure the `.htaccess` or config allows serving static files.
4. Access via `http://your-domain.com/budget/budget.html`.

### Option C: Simple Python Server (Testing)

For quick testing, you can just serve the folder using Python.

```bash
cd /path/to/budget-frontend-folder
python3 -m http.server 8080
```

---

## Part 3: HTTPS & PWA (Required for Mobile App)

To install the app as a PWA on iOS/Android, you must use **HTTPS**.

### 1. Generate Certificates

Use Certbot to get free SSL certificates from Let's Encrypt.

```bash
sudo apt install certbot
sudo certbot certonly --standalone -d your-domain.com
```

### 2. Configure Backend for HTTPS

The Go backend has built-in HTTPS support on port **8911**.

1. Symlink the certificates to the budget directory:
   
   ```bash
   sudo ln -sf /etc/letsencrypt/live/your-domain.com/fullchain.pem /opt/budget/cert.pem
   sudo ln -sf /etc/letsencrypt/live/your-domain.com/privkey.pem /opt/budget/key.pem
   ```

2. Restart the budget service:
   
   ```bash
   sudo systemctl restart budget
   ```

3. Ensure port **8911** is open in your firewall (`sudo ufw allow 8911`).

### 3. Accessing the App

 Navigate to: `https://your-domain.com:8911/budget/budget.html` (if serving static files alongside) OR ensuring your Web Server (Nginx/Apache) handles SSL and serves the HTML.

---

## Part 4: Mobile Installation (PWA)

Once your server is running with HTTPS, your users can "install" the website as an app on their phones.

### iOS (iPhone/iPad)
1.  Open Safari.
2.  Navigate to your app URL (e.g., `https://your-domain.com/budget/budget.html`).
3.  Tap the **Share** button (box with an arrow pointing up) at the bottom.
4.  Scroll down and tap **Add to Home Screen**.
5.  Tap **Add**.
6.  The app icon will appear on the home screen.

### Android (Chrome)
1.  Open Chrome.
2.  Navigate to your app URL.
3.  Tap the **Menu** button (three dots) in the top-right corner.
4.  Tap **Install App** or **Add to Home Screen**.
5.  Tap **Install** to confirm.
6.  The app will be installed and appear in your app drawer/home screen.
