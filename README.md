# Vera 📝

Vera is a professional, high-performance desktop application for generating business quotations. By leveraging the power of Go for the backend and React for the frontend, Vera provides a lightning-fast, native experience with a modern, responsive interface.

## ✨ Features

- **Automated Quotation Generation** — Quickly create structured professional quotes
- **Native Performance** — Built with Wails, ensuring a lightweight footprint on your OS
- **Modern UI/UX** — Styled with Tailwind CSS for a clean, minimalist aesthetic
- **Type Safety** — Full TypeScript integration across the frontend
- **Real-time Preview** — See changes to your quotations instantly as you type

## 🛠️ Tech Stack

| Technology | Description |
|------------|-------------|
| Framework | Wails v2 |
| Frontend Library | React |
| Bundler | Vite |
| Styling | Tailwind CSS |
| Languages | Go (Golang) & TypeScript |

## 🚀 Getting Started

### Prerequisites

Before you begin, ensure you have the following installed:

- [Go](https://go.dev/) (1.18+)
- [Node.js](https://nodejs.org/) 22.23.4, 24.22.1+, or 26.12.1+ (see `.nvmrc`)
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`

### Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/your-username/vera.git
   cd vera
   ```

2. Install frontend dependencies:
   ```bash
   cd frontend
    npm install
    ```

3. Configure local development (optional — packaged builds need no configuration):
   ```bash
   cp .env.example .env
   ```

   `wails dev` reads only `VERA_API_URL` from the gitignored project `.env`, or
   from an exported value (which wins), and requires a loopback `http(s)` host.
   Packaged and debug-packaged builds ignore that file and every exported
   override: they always use the production API. The desktop holds no database
   credentials; all data flows through `veraserver` over TLS; see
   `docs/api-deploy.md`.

4. Configure PostgreSQL before starting the **server** (not the desktop app):
   ```bash
   export VERA_POSTGRES_DSN='postgres://USER:PASSWORD@HOST:5432/DATABASE?sslmode=require'
   export VERA_POSTGRES_SCHEMA='vera'
   ```

   The server runs versioned migrations in the configured schema at
   startup. Keep the real DSN in the process environment or a local ignored
   `.env` file; never commit credentials. `.env.example` documents the
   required variable names.

## 💻 Development

Vera supports a seamless live-development workflow.

1. **Start the local API** — in one terminal, from the project root:
   ```bash
   scripts/dev-api.sh
   ```
   It sources the server-owned `.env` and forces `VERA_API_ADDR=127.0.0.1:8081`;
   Cloud Run's compiled default stays `:8080`. The desktop never starts the
   database or the server. It also passes `--local-development`, so the server
   refuses a remote, multi-host, or unparseable Postgres target before opening
   the store; only `localhost`, a loopback address, or a local Unix socket is
   accepted.

2. **Run Wails in Dev Mode** — in a second terminal, from the project root:
   ```bash
   wails dev
   ```
   The development build reads `VERA_API_URL` from the exported environment or
   `.env`, accepts only a loopback `http(s)` host, and fails clearly when neither
   supplies one instead of falling back to production. Restarting follows the
   same two steps.

3. **Frontend Hot Reloading** — In a separate terminal, navigate to the frontend directory:
   ```bash
   cd frontend
   npm run dev
   ```
   The dev server usually runs on http://localhost:34115.

## 📦 Building for Production

To create a redistributable, production-ready package for your current platform:

```bash
wails build
```

The compiled binary will be available in the `build/bin/` directory. It always
talks to the production API, even when `VERA_API_URL` is exported in the shell
that launches it; that override belongs to `wails dev` only.

## 📄 License

Distributed under the MIT License. See [LICENSE](./LICENSE) for more information.

---

Built with ❤️ using Wails, React, and Tailwind CSS.
