# 反向代理

`Scope Status` 不做 TLS 与限流，交给反代。只有一件事必须配对：**SSE 不能被缓冲**，否则页面会「一直没更新」。

## nginx

```nginx
server {
    listen 443 ssl;
    server_name friends.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE：关掉响应缓冲，并放宽读超时（应用每 15s 发一个注释帧保活）
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 1h;
        chunked_transfer_encoding on;
    }
}
```

## Caddy

```caddyfile
friends.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1        # 立即刷出每一帧，等同于关缓冲
    }
}
```

## 应用侧要对上的设置

- `TRUSTED_PROXIES` 设成反代所在网段（默认 `127.0.0.1/32,::1/128`）。只有来自可信来源的连接，`X-Forwarded-For` 才会被采信，访问日志里的 `ip` 才是真实客户端。
- 附件上传是裸文件流，反代不要额外限制请求体大小，或把上限设得比 `MAX_UPLOAD_BYTES`（默认 1 GiB）更大。
- 反代后 `HOST=127.0.0.1` 即可；直接对内网暴露时用 `HOST=0.0.0.0`。
