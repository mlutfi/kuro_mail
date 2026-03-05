<div align="center">
  <img src="./logo.png" alt="KuroMail Logo" width="200"/>
  <h1>KuroMail</h1>
  <p>Web Email Client connected to Stalwart Mail Server.</p>
</div>

---

## 📌 Features

- **Modern UI:** Clean, responsive interface built with React and Tailwind CSS.
- **Email Management:** View, send, and organize emails with ease.
- **Authentication:** Secure login with JWT and 2FA support.
- **IMAP Integration:** Fetch and manage emails directly from your Stalwart Mail Server.
- **Dark Mode:** Toggle between light and dark themes for better readability.

## 🚀 Tech Stack

### Core
- **Email Server:** Stalwart Mail Server

### Frontend
- **Framework:** Next.js 16
- **UI & Styling:** React 19, Tailwind CSS v4, shadcn/ui
- **Icons:** Lucide React

### Backend
- **Language & Framework:** Go 1.22, Fiber v2
- **Database & Cache:** PostgreSQL 16 (pgx/v5), Redis 7
- **Authentication:** JWT, TOTP (RFC 6238) for 2FA
- **Email Protocol:** go-imap/v2

---

## 🛠️ Installation Guide

### Prerequisites
Make sure you have the following installed on your machine:
- [Node.js](https://nodejs.org/) (v20+ recommended)
- [Go](https://golang.org/) (v1.22+)
- [PostgreSQL](https://www.postgresql.org/) (v16+)
- [Redis](https://redis.io/) (v7+)
- A running instance of Stalwart Mail Server

### 1. Clone the Repository
```bash
git clone https://github.com/mlutfi/kuro_mail.git
cd kuro_mail
```

### 2. Backend Setup
1. Navigate to the backend directory:
   ```bash
   cd backend
   ```
2. Set up environment variables:
   ```bash
   cp .env.example .env
   # Edit .env with your PostgreSQL, Redis, and Stalwart IMAP credentials
   ```
3. Set up the Database

4. Install dependencies and run the server (migrations will auto-run):
   ```bash
   go mod tidy
   go run ./cmd/server
   ```

### 3. Frontend Setup
1. Open a new terminal and navigate to the frontend directory:
   ```bash
   cd frontend
   ```
2. Set up environment variables:
   ```bash
   cp .env.local.example .env.local
   # Update the NEXT_PUBLIC_API_URL if needed
   ```
3. Install dependencies:
   ```bash
   npm install
   # or yarn install / pnpm install
   ```
4. Run the development server:
   ```bash
   npm run dev
   ```
5. Open [http://localhost:3000](http://localhost:3000) in your browser to see KuroMail in action.

---

## 🏗️ Project Structure
- `/backend`: Go REST API, handling migrations, IMAP connections, and WebSocket/SSE events.
- `/frontend`: Next.js frontend.
