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
- [Node.js](https://nodejs.org/) (LTS recommended)
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

## 💻 Development

Vera supports a seamless live-development workflow.

1. **Run Wails in Dev Mode** — In the project root, run:
   ```bash
   wails dev
   ```
   This handles the Go backend and bridges communication with the frontend.

2. **Frontend Hot Reloading** — In a separate terminal, navigate to the frontend directory:
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

The compiled binary will be available in the `build/bin/` directory.

## 📄 License

Distributed under the MIT License. See [LICENSE](./LICENSE) for more information.

---

Built with ❤️ using Wails, React, and Tailwind CSS.
