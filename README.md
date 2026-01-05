# Shared Budget PWA

A lightweight, self-hosted Progressive Web App (PWA) for simple shared budget tracking. Designed for couples or small groups to track "pocket money" or shared expenses with zero friction.

## Features

- **Super Simple**: Just a balance and a "Spend" button.
- **Shared State**: Real-time synchronization across devices (everyone sees the same balance).
- **Offline Capable**: Works offline and syncs when connection is restored (PWA).
- **Mobile First**: looks and feels like a native app on iOS and Android.
- **Self-Hosted**: You own your data. Database is a simple binary file storing the value left in your budget.
- **Logging:** The server keeps a log of your transations and of attempted unauthorised connections.

## Tech Stack

- **Backend**: Go (Golang) - High performance, single binary, thread-safe.
- **Frontend**: Vanilla HTML/JS/CSS - No frameworks, no build steps required for the frontend.
- **Protocol**: HTTP/HTTPS + JSON API.

## Project Structure

- `main.go`: The complete backend server.
- `budget/`: The frontend source code (HTML, CSS, JS, Service Worker).
- `users.example`: Template for the user allowlist.

## Quick Start

1. **Run the Server**:
   
   ```bash
   # Create the users file first
   cp users.example users
   # Add your name to 'users' file
   
   # Run
   go run main.go
   ```

2. **Open Client**:
   Open `budget/budget.html` in your browser.

For full production deployment instructions, see [DEPLOY.md](DEPLOY.md).

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## License

MIT
