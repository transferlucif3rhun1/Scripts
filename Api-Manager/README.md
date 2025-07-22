# API Key Management System

A comprehensive system for generating, managing, and monitoring API keys with a clean, modern UI.

## Features

- **API Key Management**
  - Generate keys with customizable parameters
  - View and filter existing keys
  - Tag-based organization
  - Bulk operations for multiple keys
  - Key templates for consistent generation

- **Monitoring & Logs**
  - System statistics dashboard
  - Filterable logs viewer
  - Performance metrics

- **User Experience**
  - Responsive design with animations
  - Dark/light theme support
  - Customizable preferences
  - Real-time connection status

## Technology Stack

- **Backend**
  - Go
  - Gin web framework
  - MongoDB

- **Frontend**
  - React
  - TypeScript
  - TailwindCSS
  - Framer Motion

## Getting Started

### Prerequisites

- Go 1.16+
- Node.js 16+
- MongoDB 4.4+

### Installation

#### Option 1: Manual Setup

1. **Start MongoDB**

   ```bash
   mongod --dbpath /path/to/your/data/directory
   ```

2. **Run the Backend**

   ```bash
   cd /path/to/project
   go run main.go
   ```

3. **Install Frontend Dependencies**

   ```bash
   cd /path/to/project
   npm install
   ```

4. **Start the Frontend**

   ```bash
   npm start
   ```

#### Option 2: Docker Compose

```bash
docker-compose up -d
```

### Default Access

- **URL**: http://localhost:3000
- **Default Login**: admin / admin123

## Configuration

### Backend Configuration

Configuration is stored in `server.json`:

```json
{
  "serverPort": "8080",
  "mongoURI": "mongodb://localhost:27017",
  "databaseName": "apiKeyManager",
  "logRetention": 30
}
```

Environment variables override file settings:

- `SERVER_PORT`
- `MONGO_URI`
- `DB_NAME`

### Frontend Configuration

The API endpoint can be configured in the Settings page or by modifying the default in `/src/api.ts`.

## Project Structure

```
/
├── src/                    # Frontend source code
│   ├── App.tsx             # Main application component
│   ├── api.ts              # API service layer
│   ├── auth.ts             # Authentication utilities
│   ├── types.ts            # TypeScript definitions
│   ├── ui.ts               # UI components
│   ├── styles.css          # Global styles
│   ├── GeneratePage.tsx    # Key generation page
│   ├── LoginPage.tsx       # Login page
│   ├── ManagePage.tsx      # Key management page
│   └── SettingsPage.tsx    # Settings page
├── main.go                 # Backend server
├── server.json             # Server configuration
└── docker-compose.yml      # Docker Compose configuration
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [Go](https://golang.org/)
- [React](https://reactjs.org/)
- [MongoDB](https://www.mongodb.com/)
- [TailwindCSS](https://tailwindcss.com/)