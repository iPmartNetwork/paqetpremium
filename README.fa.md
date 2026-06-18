# PaqetPremium

<p align="center">
  <strong>تونل سطح‌پکت برای VPS لینوکسی</strong> — libpcap + KCP/QUIC + smux.<br>
  <a href="README.md">English</a> · <a href="README.fa.md">فارسی</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-blue" alt="platform">
  <img src="https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="go">
  <img src="https://img.shields.io/badge/transport-KCP%20%7C%20QUIC-success" alt="transport">
  <img src="https://img.shields.io/badge/version-0.9.0-informational" alt="version">
  <img src="https://img.shields.io/badge/license-GPL--3.0-blue" alt="license">
</p>

---

PaqetPremium ترافیک را داخل **پکت‌های TCP خام ساختگی** روی یک اینترفیس لینوکسی
(از طریق libpcap) جابه‌جا می‌کند، سپس از **KCP** یا **QUIC** به‌عنوان پروتکل
حامل و از **smux** برای multiplex کردن استریم‌ها استفاده می‌کند. این پروژه برای
استقرار دو-نودی طراحی شده است:

| نقش | محل | مقدار `role` | کاربرد |
|------|------|---------------|---------|
| ورودی | VPS ایران | `client` | port-forward و SOCKS5 به سمت سرویس‌ها |
| خروجی | VPS خارج (Kharej) | `server` | رله ترافیک تونل به اینترنت آزاد |

> ویندوز، مک و کلاینت دسکتاپ **خارج از scope** هستند. دستور `run` باید روی لینوکس
> و با دسترسی root اجرا شود (raw socket / pcap).

## معماری

```
   کاربر / اینترنت        VPS ایران (role: client)
        ───────────►   ┌───────────────────────────────┐
                       │  port-forward (TCP/UDP)        │
                       │  SOCKS5 (CONNECT + UDP ASSOC.) │
                       │            │                   │
                       │      pcap (TCP ساختگی)         │
                       │            │                   │
                       │   KCP / QUIC  →  smux          │
                       └──────────────┬────────────────┘
                                      │  تونل
                       ┌──────────────▼────────────────┐
                       │  VPS خارج (role: server)       │
                       │  رله به مقصد نهایی             │
                       │  iptables + ip6tables          │
                       └────────────────────────────────┘
```

## قابلیت‌ها

- **دو ترنسپورت** — KCP (پیش‌فرض، بهینه برای لینک‌های پرافت‌وخیز) یا QUIC (TLS 1.3)، قابل انتخاب در کانفیگ.
- **احراز هویت دوطرفه** — هر دو ترنسپورت peer را بر اساس secret مشترک احراز می‌کنند. QUIC یک گواهی قطعی مشتق‌شده از secret را در هر دو طرف pin می‌کند.
- **Port forwarding** — TCP و UDP، با امکان bind هر rule به upstream مشخص.
- **SOCKS5** — CONNECT (TCP) و UDP ASSOCIATE، با احراز هویت اختیاری کاربر/رمز.
- **چند upstream** — استراتژی‌های `failover`، `round_robin`، `weighted`، `least_latency` همراه با health check و failover خودکار.
- **Hot reload** — کلاینت (upstream + forward + SOCKS5) و سرور (کانفیگ + فایروال) از طریق `SIGHUP` یا Admin API.
- **IPv4 و IPv6 اختیاری** روی همان مسیر TCP ساختگی.
- **Admin API و متریک** — health، status، reload و متریک Prometheus، با احراز هویت توکن اختیاری.
- **یکپارچه با systemd** — سرویس تکی یا چند instance نام‌دار کلاینت.

## پیش‌نیازها

- لینوکس (amd64 یا arm64) با دسترسی root.
- هدرهای `libpcap` و یک کامپایلر C (build از **CGO** استفاده می‌کند).
- `iptables` / `ip6tables` روی نود سرور.

## نصب سریع

بوت‌استرپ تک‌خطی (clone، build و اجرای نصب‌کننده تعاملی):

```bash
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash
```

اجرای مستقیم یک مسیر مشخص:

```bash
# VPS خارج (خروجی)
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash -s -- server

# VPS ایران (ورودی)
curl -fsSL https://raw.githubusercontent.com/iPmartNetwork/paqetpremium/master/scripts/install-linux.sh | sudo bash -s -- client
```

یا clone کنید و نصب‌کننده/مدیر را مستقیم اجرا کنید:

```bash
git clone https://github.com/iPmartNetwork/paqetpremium
cd paqetpremium
sudo ./install-premium.sh            # منوی تعاملی
```

نصب‌کننده اینترفیس/IP/MAC را تشخیص می‌دهد، وابستگی‌ها (و در صورت نیاز یک نسخه
به‌روز Go) را نصب می‌کند، باینری را می‌سازد، کانفیگ را می‌نویسد و systemd را
راه‌اندازی می‌کند — به‌همراه health check پس از استارت.

## ساخت دستی

```bash
sudo apt install -y libpcap-dev        # دبیان/اوبونتو
make build-linux-amd64                 # یا: build-linux-arm64
# معادل دستی:
CGO_ENABLED=1 go build -o paqetpremium ./cmd/paqetpremium
```

خارج از لینوکس فقط می‌توانید کانفیگ را اعتبارسنجی کنید (بدون pcap):

```bash
go build -o paqetpremium ./cmd/paqetpremium
./paqetpremium test -c example/client.yaml
```

## خط فرمان (CLI)

```bash
paqetpremium run    -c config.yaml   # اجرای تونل (لینوکس + root)
paqetpremium test   -c config.yaml   # اعتبارسنجی کانفیگ (+ تست زنده روی لینوکس)
paqetpremium bench  -c client.yaml   # اندازه‌گیری latency آپ‌استریم‌ها (لینوکس)
paqetpremium reload -c client.yaml   # hot reload از طریق Admin API
paqetpremium version
```

## مدیریت سرویس

نصب‌کننده دستورات مدیریتی را هم فراهم می‌کند:

```bash
sudo ./install-premium.sh status            # سرویس‌ها + وضعیت admin
sudo ./install-premium.sh logs   client     # دنبال‌کردن لاگ (server|client|<tunnel>)
sudo ./install-premium.sh reload client      # hot reload با SIGHUP
sudo ./install-premium.sh restart server
sudo ./install-premium.sh update             # build مجدد از repo و restart
sudo ./install-premium.sh add-tunnel         # افزودن instance نام‌دار کلاینت
sudo ./install-premium.sh uninstall
```

یونیت‌های معادل systemd: `paqetpremium-server.service`،
`paqetpremium-client.service` و یونیت تمپلیتی `paqetpremium-client@<name>.service`.

## پیکربندی

هر دو طرف باید روی **یک** `transport.protocol` و **یک** secret مشترک توافق داشته باشند.

### KCP (پیش‌فرض)

```yaml
transport:
  protocol: kcp
  conn: 6
  kcp:
    mode: fast
    block: aes-128-gcm
    key: SHARED_SECRET
    mtu: 1150
```

### QUIC

```yaml
transport:
  protocol: quic
  conn: 6
  kcp:
    key: SHARED_SECRET    # secret مشترک (همان فیلد KCP)
  quic:
    alpn: paqetpremium
    idle_timeout: 30s
    max_idle_timeout: 60s
```

### چند upstream

```yaml
upstream:
  strategy: failover        # failover | round_robin | weighted | least_latency
  health_check:
    interval: 10s
    timeout: 3s
    fail_threshold: 3
    recover_threshold: 2
  servers:
    - name: de-fra-1
      addr: 45.1.1.1:8888
      key: SHARED_SECRET
      priority: 1
      weight: 3
    - name: nl-ams-1
      addr: 45.2.2.2:8888
      key: SHARED_SECRET
      priority: 2
```

### SOCKS5 (TCP + UDP)

```yaml
socks5:
  - listen: "127.0.0.1:1080"
    # اختیاری:
    # auth: { user: alice, pass: secret }
```

### IPv6 (اختیاری)

```yaml
network:
  interface: eth0
  ipv4:
    addr: "10.0.0.5:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
  ipv6:
    addr: "[2001:db8::5]:0"
    router_mac: "aa:bb:cc:dd:ee:ff"
```

برای نمونه‌های کامل به پوشه `example/` مراجعه کنید: `client.yaml`، `server.yaml`، `client-quic.yaml` و `server-quic.yaml`.

## Admin API

وقتی `admin.listen` تنظیم شده باشد فعال است:

| مسیر | متد | توضیح |
|----------|--------|-------------|
| `/healthz` | GET | بررسی liveness |
| `/api/v1/status` | GET | وضعیت JSON (role، upstreamها، session‌ها) |
| `/api/v1/reload` | POST | بارگذاری مجدد کانفیگ از دیسک |
| `/metrics` | GET | متریک Prometheus (`admin.metrics: true`) |

با تنظیم `admin.token` مسیرهای `/api/v1/*` و `/metrics` محافظت می‌شوند (نه
`/healthz`). توکن را به‌صورت `Authorization: Bearer <token>` یا `?token=<token>`
ارسال کنید.

## نکات امنیتی

- secret مشترک نقطه اتکای اعتماد است. KCP کلید رمز بلاکی خود را از آن مشتق می‌کند؛ QUIC گواهی مشتق‌شده‌ی قطعی از آن را در **هر دو** طرف pin می‌کند. از یک secret قوی و یکتا استفاده کنید و فایل‌های کانفیگ را فقط برای root قابل‌خواندن نگه دارید (`0640`).
- Admin API به‌صورت پیش‌فرض روی `127.0.0.1` گوش می‌دهد. اگر آن را در دسترس عموم قرار دادید، حتماً `admin.token` را تنظیم کنید.
- سرور قوانین `iptables`/`ip6tables` (NOTRACK + drop RST) را روی پورت تونل اعمال می‌کند؛ مطمئن شوید سیاست فایروال شما این پورت را مجاز می‌کند.

## ساختار پروژه

```
cmd/paqetpremium/     نقطه ورود CLI
internal/
  app/                حلقه اجرا، helperهای test/bench/reload
  config/             کانفیگ YAML و اعتبارسنجی
  netutil/            TCP flags و helperهای آدرس
  pcap/               موتور پکت خام لینوکس (libpcap)
  transport/          نشست‌های KCP + QUIC + smux
  tunnel/             اجراکننده‌های client/server/relay
  tunnelpool/         pool چند نشست
  upstream/           مدیر چند سرور + health
  forward/            port forwarding برای TCP/UDP
  socks5/             SOCKS5 (TCP + UDP)
  iptables/           قوانین فایروال سرور
  admin/              HTTP API + متریک
  metrics/            شمارنده‌ها + Prometheus
  protocol/           پیام‌های کنترلی تونل
  platform/           محدودیت‌های استقرار لینوکس
  version/            متادیتای build
example/              کانفیگ‌های YAML آماده ویرایش
install-premium.sh    نصب‌کننده و مدیر
scripts/install-linux.sh   بوت‌استرپ تک‌خطی
```

هدف‌ها: **linux/amd64**، **linux/arm64**.

## وضعیت و نقشه راه

پیاده‌سازی هسته کامل است و مسیرهای مستقل از پلتفرم unit-test شده‌اند؛ تست
end-to-end روی VPSهای واقعی تنها قدم باقی‌مانده پیش از تگ `1.0.0` است.

- [x] موتور pcap، ترنسپورت KCP، هندشیک ping
- [x] port-forward، SOCKS5، session pool، iptables
- [x] چند upstream، health check، hot reload
- [x] Admin API، متریک، IPv6، نصب‌کننده
- [x] CLI برای reload/bench، احراز هویت admin، arm64
- [x] ترنسپورت QUIC با pinning دوطرفه گواهی
- [ ] اعتبارسنجی زنده روی VPS و انتشار `1.0.0`

برای یادداشت‌های انتشار به [CHANGELOG.md](CHANGELOG.md) مراجعه کنید.

## مجوز

تحت **مجوز GNU General Public License v3.0 (GPL-3.0)** منتشر شده است. متن کامل در فایل [LICENSE](LICENSE) موجود است.
