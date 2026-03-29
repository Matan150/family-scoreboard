import uvicorn

from app import app

PORT = 8080

if __name__ == "__main__":
    print(f"🚀 Server running at http://localhost:{PORT}")
    uvicorn.run(app, host="0.0.0.0", port=PORT)
