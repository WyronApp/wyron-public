# 🐍 Wyron Python Client

Official Python SDK for **Wyron Panel** (REST + gRPC).

این کتابخانه امکان اتصال به پنل Wyron از طریق:

- ✅ REST API
- ✅ gRPC API

را فراهم می‌کند.

---

## 🚀 نصب

### نصب مستقیم از GitHub

```bash
pip install git+https://github.com/WyronApp/wyron-public.git#subdirectory=python-client
```

## 🐍 Quick Usage

```python
from wyron_client import WyronClient

# Connect to panel
client = WyronClient(
    base_url="https://panel.example.com",
    username="admin",
    password="your-password"
)

# List users
for user in client.list_users(limit=5):
    print(user.user_key, user.usage)

# Create a new user
user = client.create_user({
    "user_key": "user123",
    "social_id": 123456789,
    "duration_seconds": 2592000,  # 30 days
    "traffic_limit": 1073741824,  # 1GB
    "server_access": [
        {"server_id": "router-1", "interfaces": ["wg0"]}
    ]
})

print("Created:", user.user_key)
```

## ⚡ gRPC Usage

```python
from wyron_client import WyronGrpcClient

# Connect to gRPC service (example: port 50051)
client = WyronGrpcClient(
    host="panel.example.com:50051",  # gRPC runs on a separate port
    username="admin",
    password="your-password",
    secure=False  # insecure channel (no TLS)
)

# Fetch metrics
metrics = client.metrics()
print(metrics)

# List servers
for server in client.list_servers():
    print(server.name)
```
