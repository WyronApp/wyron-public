import httpx
import grpc
from dataclasses import dataclass, field
from typing import List, Optional, Dict, Any, Protocol
import pyqrcode
from wyron_proto import auth_pb2
from wyron_proto import auth_pb2_grpc
from wyron_proto import server_pb2
from wyron_proto import server_pb2_grpc
from wyron_proto import user_pb2
from wyron_proto import user_pb2_grpc
from google.protobuf.empty_pb2 import Empty


# =========================================================
# Exceptions
# =========================================================

class InterfaceNotFound(Exception):
    pass


class InterfaceMissingKey(Exception):
    pass


# =========================================================
# Models
# =========================================================

@dataclass
class WireGuardInterface:
    name: str
    display_name: Optional[str] = None
    subnet: Optional[str] = None
    endpoint: Optional[str] = None
    dns: Optional[str] = None
    port: Optional[int] = None
    created_at: Optional[int] = None
    public_key: Optional[str] = None


@dataclass
class Server:
    name: str
    address: str
    username: str
    display_name: Optional[str] = None
    created_at: Optional[int] = None
    interfaces: List[WireGuardInterface] = field(default_factory=list)

class ServerResolver(Protocol):
    def get_server(self, server_id: str) -> Server: ...

@dataclass
class PeerState:
    server_id: str
    interface: str
    allowed_address: str
    private_key: Optional[str] = None

    _client: Optional[ServerResolver] = field(default=None, repr=False)

    def bind_client(self, client: ServerResolver):
        self._client = client

    def _resolve_server(self) -> Server:

        if not self._client:
            raise InterfaceNotFound("Peer has no client bound")

        return self._client.get_server(self.server_id)


    def _resolve_iface(self) -> WireGuardInterface:
        server = self._resolve_server()

        iface = next(
            (i for i in server.interfaces if i.name == self.interface),
            None
        )

        if not iface:
            raise InterfaceNotFound(
                f"Interface '{self.interface}' not found on server '{self.server_id}'"
            )

        return iface

    def generate_config(self) -> str:
        if not self.private_key:
            raise InterfaceMissingKey("Peer private_key missing")

        iface = self._resolve_iface()

        if not iface.endpoint:
            raise InterfaceMissingKey("Interface endpoint missing")

        if not iface.public_key:
            raise InterfaceMissingKey("Interface public_key missing")

        return f"""[Interface]
    Address = {self.allowed_address}
    DNS = {iface.dns}
    PrivateKey = {self.private_key}

[Peer]
    AllowedIPs = 0.0.0.0/0
    Endpoint = {iface.endpoint}:{iface.port}
    PublicKey = {iface.public_key}
    """

    def save_qr(self, file_path: str, scale: int = 6) -> None:
        # QR contains the config text exactly

        config = self.generate_config()
        qr = pyqrcode.create(config)
        qr.png(file_path, scale=scale)


@dataclass
class User:
    user_key: str
    sub_token: str
    social_id: int
    active: bool
    traffic_limit: int
    usage: int
    duration_seconds: int
    created_at: int
    first_connected_at: int
    last_connected_at: int
    created_by: str

    peers: List[PeerState] = field(default_factory=list)


# =========================================================
# Client
# =========================================================

class WyronClient:
    def __init__(self, base_url: str, username: str, password: str, timeout: int = 15):
        self.base_url = base_url.rstrip("/") + "/api"
        self.username = username
        self.password = password
        self.token: Optional[str] = None
        self.timeout = timeout
        self.session = httpx.Client(
            http2=True,
            timeout=httpx.Timeout(timeout),
            limits=httpx.Limits(
                max_connections=50,
                max_keepalive_connections=20,
            ),
        )

        self.login()

    def _headers(self) -> Dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def _request(self, method: str, path: str, **kwargs) -> Dict[str, Any]:
        url = f"{self.base_url}{path}"

        response = self.session.request(
            method,
            url,
            headers=self._headers(),
            timeout=self.timeout,
            **kwargs,
        )

        # Auto re-login on 401
        if response.status_code == 401:
            self.login()
            response = self.session.request(
                method,
                url,
                headers=self._headers(),
                timeout=self.timeout,
                **kwargs,
            )

        response.raise_for_status()

        return response.json()

    # ---------------------------
    # Auth
    # ---------------------------

    def login(self) -> None:
        r = self.session.post(
            f"{self.base_url}/auth/login",
            json={"username": self.username, "password": self.password},
            timeout=self.timeout,
        )

        r.raise_for_status()

        token = r.json().get("token")
        if not token:
            raise Exception("Login failed: token missing in response")
        self.token = token

    # ---------------------------
    # Parsing helpers
    # ---------------------------

    @staticmethod
    def _parse_server(s: Dict[str, Any]) -> Server:
        interfaces = [WireGuardInterface(**iface) for iface in s.get("interfaces", [])]
        s2 = dict(s)
        s2["interfaces"] = interfaces
        return Server(**s2)

    @staticmethod
    def _parse_user(u: Dict[str, Any]) -> User:
        peers = [PeerState(**peer) for peer in u.get("peers", [])]
        u2 = dict(u)
        u2["peers"] = peers
        return User(**u2)

    # =========================================================
    # Servers
    # =========================================================

    def list_servers(self) -> List[Server]:
        data = self._request("GET", "/servers")
        return [self._parse_server(s) for s in data.get("data", [])]

    def get_server(self, server_id: str) -> Server:
        data = self._request("GET", f"/servers/{server_id}")
        return self._parse_server(data["data"])

    def create_or_update_server_raw(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("POST", "/servers", json=payload)

    def create_or_update_server(self, payload: Dict[str, Any]) -> Server:
        res = self.create_or_update_server_raw(payload)
        name = res.get("name")
        if not name:
            raise Exception("server create/update succeeded but 'name' missing")
        return self.get_server(name)

    def delete_server(self, server_id: str) -> Dict[str, Any]:
        return self._request("DELETE", f"/servers/{server_id}")

    def update_interface(self, server_id: str, payload: Dict[str, Any]) -> Dict[str, Any]:
        return self._request("POST", f"/servers/{server_id}/interfaces", json=payload)

    def delete_interface(self, server_id: str, interface_name: str) -> Dict[str, Any]:
        return self._request("DELETE", f"/servers/{server_id}/interfaces/{interface_name}")

    # =========================================================
    # Users
    # =========================================================

    def list_users(
        self,
        social_id: Optional[int] = None,
        status: Optional[str] = None,
        search: Optional[str] = None,
        limit: int = 50,
        skip: int = 0,
        sort: str = "created_at",
        order: str = "desc",
    ) -> List[User]:
        params: Dict[str, Any] = {
            "limit": limit,
            "skip": skip,
            "sort": sort,
            "order": order,
        }
        if social_id is not None:
            params["social_id"] = social_id
        if status:
            params["status"] = status
        if search:
            params["search"] = search

        data = self._request("GET", "/users", params=params)

        users: List[User] = []
        for u in data.get("result", []):
            user = self._parse_user(u)
            for peer in user.peers:
                peer.bind_client(self)
            users.append(user)
        return users


    def get_user(self, user_id: str) -> User:
        data = self._request("GET", f"/users/{user_id}")
        user = self._parse_user(data["result"])
        if user.peers:
            for peer in user.peers:
                peer.bind_client(self)

        return user

    def create_user(self, payload: Dict[str, Any]) -> User:
        data = self._request("POST", "/users", json=payload)
        user = self._parse_user(data["result"])
        if user.peers:
            for peer in user.peers:
                peer.bind_client(self)

        return user

    def edit_user(self, user_id: str, payload: Dict[str, Any]) -> User:
        data = self._request("PATCH", f"/users/{user_id}", json=payload)
        user = self._parse_user(data["result"])
        for peer in user.peers:
            peer.bind_client(self)
        return user

    def delete_user(self, user_id: str) -> Dict[str, Any]:
        return self._request("DELETE", f"/users/{user_id}")

    def enable_user(self, user_id: str) -> Dict[str, Any]:
        return self._request("POST", f"/users/{user_id}/enable")

    def disable_user(self, user_id: str) -> Dict[str, Any]:
        return self._request("POST", f"/users/{user_id}/disable")

    def reset_usage(self, user_id: str) -> Dict[str, Any]:
        return self._request("POST", f"/users/{user_id}/reset-usage")

    def metrics(self) -> Dict[str, Any]:
        return self._request("GET", "/users/metrics")

    def me(self) -> Dict[str, Any]:
        return self._request("GET", "/auth/me")

    def logout(self) -> Dict[str, Any]:
        return self._request("POST", "/auth/logout")


class WyronGrpcClient:
    def __init__(
        self,
        host: str,
        username: str,
        password: str,
        timeout: int = 15,
        secure: bool = False,
    ):
        self.host = host
        self.username = username
        self.password = password
        self.timeout = timeout
        self.token: Optional[str] = None

        if secure:
            creds = grpc.ssl_channel_credentials()
            self.channel = grpc.secure_channel(host, creds)
        else:
            self.channel = grpc.insecure_channel(host)

        self.auth = auth_pb2_grpc.AuthServiceStub(self.channel)
        self.server = server_pb2_grpc.ServerServiceStub(self.channel)
        self.user = user_pb2_grpc.UserServiceStub(self.channel)

        self.login()

    # ---------------------------
    # Internal helpers
    # ---------------------------

    def _metadata(self):
        if not self.token:
            return []
        return [("authorization", f"Bearer {self.token}")]

    def _call(self, func, request):
        try:
            return func(
                request,
                metadata=self._metadata(),
                timeout=self.timeout,
            )
        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.UNAUTHENTICATED:
                self.login()
                return func(
                    request,
                    metadata=self._metadata(),
                    timeout=self.timeout,
                )
            raise

    # ---------------------------
    # Auth
    # ---------------------------

    def login(self):
        response = self.auth.Login(
            auth_pb2.LoginRequest(
                username=self.username,
                password=self.password,
            ),
            timeout=self.timeout,
        )
        self.token = response.token

    # =========================================================
    # Servers
    # =========================================================

    def list_servers(self) -> List[Server]:
        response = self._call(self.server.List, Empty())
        return [self._parse_server(s) for s in response.servers]

    def get_server(self, server_id: str) -> Server:
        response = self._call(
            self.server.Get,
            server_pb2.ServerIDRequest(id=server_id),
        )
        return self._parse_server(response)

    def create_or_update_server(self, payload: Dict[str, Any]) -> Server:
        response = self._call(
            self.server.Update,
            server_pb2.UpdateServerRequest(**payload),
        )
        return self._parse_server(response)

    def delete_server(self, server_id: str):
        self._call(
            self.server.Delete,
            server_pb2.ServerIDRequest(id=server_id),
        )

    def update_interface(
        self,
        server_id: str,
        name: str,
        display_name: str,
        endpoint: str,
        dns: str,
    ) -> WireGuardInterface:
        request = server_pb2.InterfaceRequest(
            server_id=server_id,
            name=name,
            display_name=display_name,
            endpoint=endpoint,
            dns=dns,
        )
        response = self._call(self.server.UpdateInterface, request)
        i = response.interface
        return WireGuardInterface(
            name=i.name,
            display_name=i.display_name,
            subnet=i.subnet,
            endpoint=i.endpoint,
            dns=i.dns,
            port=i.port,
            created_at=i.created_at,
            public_key=i.public_key,
        )

    def delete_interface(self, server_id: str, name: str) -> None:
        request = server_pb2.InterfaceRequest(server_id=server_id, name=name)
        self._call(self.server.DeleteInterface, request)

    # =========================================================
    # Users
    # =========================================================

    def list_users(
        self,
        social_id: Optional[int] = None,
        status: Optional[str] = None,
        search: Optional[str] = None,
        limit: int = 50,
        skip: int = 0,
        sort: str = "created_at",
        order: str = "desc",
    ) -> List[User]:

        request = user_pb2.ListUsersRequest(
            limit=limit,
            skip=skip,
            sort=sort,
            order=order,
        )

        if social_id is not None:
            request.social_id = social_id
        if status:
            request.status = status
        if search:
            request.search = search

        response = self._call(self.user.List, request)

        users: List[User] = []

        for u in response.users:
            user = self._parse_user(u)
            for peer in user.peers:
                peer.bind_client(self)
            users.append(user)

        return users

    def get_user(self, user_id: str) -> User:
        response = self._call(
            self.user.Get,
            user_pb2.UserKeyRequest(user_key=user_id),
        )

        user = self._parse_user(response)
        for peer in user.peers:
            peer.bind_client(self)
        return user

    def create_user(self, payload: Dict[str, Any]) -> User:
        request = user_pb2.CreateUserRequest(**payload)

        response = self._call(self.user.Create, request)

        user = self._parse_user(response)
        for peer in user.peers:
            peer.bind_client(self)
        return user

    def edit_user(self, user_id: str, payload: Dict[str, Any]) -> User:
        request = user_pb2.EditUserRequest(user_key=user_id, **payload)
        response = self._call(self.user.Edit, request)

        user = self._parse_user(response)
        for peer in user.peers:
            peer.bind_client(self)
        return user

    def delete_user(self, user_id: str):
        self._call(
            self.user.Delete,
            user_pb2.UserKeyRequest(user_key=user_id),
        )

    def enable_user(self, user_id: str):
        self._call(
            self.user.Enable,
            user_pb2.UserKeyRequest(user_key=user_id),
        )

    def disable_user(self, user_id: str):
        self._call(
            self.user.Disable,
            user_pb2.UserKeyRequest(user_key=user_id),
        )

    def reset_usage(self, user_id: str):
        self._call(
            self.user.ResetUsage,
            user_pb2.UserKeyRequest(user_key=user_id),
        )

    def metrics(self):
        return self._call(self.user.Metrics, Empty())

    # =========================================================
    # Parsers
    # =========================================================

    def me(self) -> Dict[str, Any]:
        res = self._call(self.auth.Me, Empty())
        return {"username": res.username}

    def create_admin(self, username: str, password: str) -> None:
        self._call(self.auth.CreateAdmin, auth_pb2.CreateAdminRequest(
            username=username, password=password
        ))

    @staticmethod
    def _parse_server(s) -> Server:
        interfaces = [
            WireGuardInterface(
                name=i.name,
                display_name=i.display_name,
                subnet=i.subnet,
                endpoint=i.endpoint,
                dns=i.dns,
                port=i.port,
                created_at=i.created_at,
                public_key=i.public_key,
            )
            for i in s.interfaces
        ]

        return Server(
            name=s.id,
            address=s.address,
            username=s.username,
            display_name=s.display_name,
            created_at=s.created_at,
            interfaces=interfaces,
        )

    @staticmethod
    def _parse_user(u) -> User:
        peers = [
            PeerState(
                server_id=p.server_id,
                interface=p.interface,
                allowed_address=p.allowed_address,
                private_key=p.private_key,
            )
            for p in u.peers
        ]

        return User(
            user_key=u.user_key,
            sub_token=u.sub_token,
            social_id=u.social_id,
            active=u.active,
            traffic_limit=u.traffic_limit,
            usage=u.usage,
            duration_seconds=u.duration_seconds,
            created_at=u.created_at,
            first_connected_at=u.first_connected_at,
            last_connected_at=u.last_connected_at,
            created_by=u.created_by,
            peers=peers,
        )
