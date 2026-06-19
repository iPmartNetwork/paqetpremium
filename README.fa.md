# PaqetPremium

<p align="center">
  <strong>تونل سطح‌پکت برای VPS لینوکسی</strong> — libpcap + KCP/QUIC + smux.<br>
  <a href="README.md">English</a> · <a href="README.fa.md">فارسی</a>
</p>

<p align="center"><a href="https://ipmartnetwork.github.io/paqetpremium/"><strong>🌐 وب‌سایت و مستندات</strong></a></p>

<p align="center">
  <img src="https://img.shields.io/badge/platform-linux%20amd64%20%7C%20arm64-blue" alt="platform">
  <img src="https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="go">
  <img src="https://img.shields.io/badge/transport-KCP%20%7C%20QUIC-success" alt="transport">
  <img src="https://img.shields.io/badge/version-0.15.0-informational" alt="version">
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
- **Port forwarding** — TCP و UDP، با امکان bind هر rule به upstream مشخص. پروتکل‌های مبتنی بر UDP (QUIC، Hysteria2، TUIC، WireGuard) روی forwarding UDP کار می‌کنند — مرز دیتاگرام‌ها به‌صورت سرتاسری حفظ می‌شود.
- **حالت رِنج شفاف همهٔ پورت‌ها** — یک listener تکی روی کلاینت کل یک رِنج پورت TCP را به‌صورت شفاف (از طریق iptables REDIRECT + `SO_ORIGINAL_DST`) به localhost سرور تونل می‌کند، بدون کانفیگ per-port و بدون هیچ تغییری در سمت سرور.
- **SOCKS5** — CONNECT (TCP) و UDP ASSOCIATE، با احراز هویت اختیاری کاربر/رمز.
- **چند upstream** — استراتژی‌های `failover`، `round_robin`، `weighted`، `least_latency` همراه با health check و failover خودکار.
- **upstream خودترمیم** — poolهای مرده (ری‌استارت سرور، اختلال شبکه، تایم‌اوت keepalive) به‌صورت خارج‌ازباند و با backoff بازسازی می‌شوند و پس از موفقیت یک ping دوباره سالم علامت می‌خورند — بدون نیاز به ری‌استارت کلاینت.
- **FEC قابل‌تنظیم برای KCP** — تصحیح خطای رو‌به‌جلوی اختیاری (`data_shard`/`parity_shard`) پکت‌های ازدست‌رفته را بدون ارسال مجدد روی لینک‌های پرافت بازیابی می‌کند؛ پنجره‌ها هم قابل تنظیم‌اند.
- **Hot reload** — کلاینت (upstream + forward + SOCKS5) و سرور (کانفیگ + فایروال) از طریق `SIGHUP` یا Admin API.
- **IPv4 و IPv6 اختیاری** روی همان مسیر TCP ساختگی.
- **Admin API، متریک و داشبورد وب** — health، status، reload، متریک Prometheus و یک صفحهٔ وضعیت زندهٔ تیره‌تم، با احراز هویت توکن اختیاری.
- **یکپارچه با systemd** — سرویس تکی یا چند instance نام‌دار کلاینت، همراه با مدیریت per-tunnel (لیست/ویرایش/حذف).

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

### نصب از طریق پکیج

ریلیزهای تگ‌خورده پکیج‌های `.deb` و `.rpm` را هم برای **amd64** و **arm64**
منتشر می‌کنند (پکیج وابستگی `libpcap` را اعلام می‌کند):

```bash
sudo dpkg -i paqetpremium_*.deb      # دبیان/اوبونتو
sudo rpm   -i paqetpremium-*.rpm     # RHEL/فدورا
```

سپس برای راه‌اندازی راهنمایی‌شده نصب‌کننده را اجرا کنید
(`sudo ./install-premium.sh`) یا مستقیماً با `paqetpremium run -c <config>` شروع
کنید. بوت‌استرپ تک‌خطی بالا همچنان روش اصلی نصب است.

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
sudo ./install-premium.sh tunnels            # لیست تونل‌های پیکربندی‌شده با جزئیات
sudo ./install-premium.sh edit   client      # ویرایش کانفیگ یک تونل و restart همان (server|client|<name>)
sudo ./install-premium.sh remove mytunnel    # حذف یک تونل (کانفیگ + سرویس)
sudo ./install-premium.sh uninstall
```

یونیت‌های معادل systemd: `paqetpremium-server.service`،
`paqetpremium-client.service` و یونیت تمپلیتی `paqetpremium-client@<name>.service`.

دستورات `tunnels`، `edit` و `remove` به‌صورت آیتم‌های منو هم در دسترس‌اند.
`tunnels` هر تونل پیکربندی‌شده را همراه نقش، ترنسپورت، خلاصهٔ
upstream/forward/socks/range و وضعیت زنده فهرست می‌کند؛ `edit` کانفیگ یک تونل را
باز و اعتبارسنجی می‌کند و تنها همان سرویس را restart می‌کند؛ و `remove` کانفیگ و
سرویس یک تونل را بدون دست‌زدن به سایر تونل‌ها یا باینری حذف می‌کند.

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
    # تصحیح خطای رو‌به‌جلوی اختیاری (پیش‌فرض خاموش):
    data_shard: 10      # FEC: بازیابی پکت‌های ازدست‌رفته بدون ارسال مجدد (پیش‌فرض 0 = خاموش)
    parity_shard: 3     # هر دو طرف باید یکسان باشند
    snd_wnd: 1024       # بازنویسی اختیاری پنجره‌ها
    rcv_wnd: 1024
```

FEC با کمی پهنای‌باند اضافه‌تر، ارسال مجدد را روی لینک‌های پرافت (رایج در مسیرهای
ایران↔خارج) به‌شدت کاهش می‌دهد: یک گروه `data_shard: 10` / `parity_shard: 3` تا ۳
پکت ازدست‌رفته را بدون رفت‌وبرگشت بازیابی می‌کند. این قابلیت **به‌صورت پیش‌فرض
خاموش** است و هر دو طرف باید از مقادیر **یکسان** `data_shard`/`parity_shard`
استفاده کنند. `snd_wnd`/`rcv_wnd` بازنویسی اختیاری پنجره‌اند؛ آن‌ها را تنظیم نکنید
تا پیش‌فرض‌های مبتنی بر نقش حفظ شوند.

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

### حالت رِنج شفاف همهٔ پورت‌ها (کلاینت)

```yaml
range:
  enabled: true
  protocol: tcp           # فقط tcp (حالت شفاف UDP در دست برنامه‌ریزی است)
  redirect_port: 47999    # listener محلی که iptables ترافیک را با REDIRECT به آن می‌فرستد
  target_host: "127.0.0.1" # هاست سمت سرور که سرویس‌ها روی آن هستند
  ports: "1-65535"        # رِنج/لیست برای تونل، مثلاً "443,8443,2000-3000"
  exclude: "22"           # هرگز این‌ها را redirect نکن (SSH را نگه دار!)
```

با فعال‌بودن حالت رِنج، هر اتصال TCP به IP کلاینت روی پورتی داخل `ports` به‌صورت
شفاف به `target_host:<پورت اصلی>` روی سرور تونل می‌شود. کلاینت یک iptables nat
REDIRECT به یک listener محلی تکی نصب می‌کند و پورت مقصد اصلی هر اتصال را از طریق
`SO_ORIGINAL_DST` بازیابی می‌کند، بنابراین **هر** پورتِ localhost سرور را از طریق
IP ورودی و بدون کانفیگ per-port در دسترس دارید — و سرور به هیچ تغییری نیاز ندارد
(رِله‌اش هم‌اکنون هدف هر اتصال را dial می‌کند). SSH (`22`) و `redirect_port`
به‌صورت خودکار مستثنا می‌شوند. نصب‌کننده این را به‌صورت گزینهٔ ویزارد **«تونل شفاف
همهٔ پورت‌های ورودی»** ارائه می‌دهد.

> **امنیت:** این کار همهٔ پورت‌های localhost سرور را از طریق IP ورودی در معرض قرار
> می‌دهد. پورت‌های حساس (دیتابیس‌ها، Admin API و غیره) را در `exclude` نگه دارید.

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

### داشبورد

سرور admin یک **صفحهٔ وضعیت زنده** و مستقلِ تیره‌تم را هم روی ریشه (`/`) سرو
می‌کند: throughput دانلود/آپلود با نرخ بر-ثانیه، session‌های فعال، شمارنده‌های
TCP/UDP و رله، تعداد خطاها، و یک جدول health/RTT/session برای هر upstream، که هر
چند ثانیه به‌صورت خودکار به‌روزرسانی می‌شود. چون admin به‌صورت پیش‌فرض روی
`127.0.0.1` گوش می‌دهد، آن را از طریق یک تونل SSH ببینید:

```bash
ssh -L 9090:127.0.0.1:9090 root@<server>
# سپس http://localhost:9090 را باز کنید  (اگر توکن admin تنظیم شده، ?token=... را اضافه کنید)
```

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

پیاده‌سازی هسته کامل، unit-test شده و توسط یک مجموعهٔ تست یکپارچهٔ end-to-end در
CI آزموده شده است؛ burn-in در دنیای واقعی روی VPSهای زنده تنها قدم باقی‌مانده پیش
از تگ پایدار `1.0.0` است.

- [x] موتور pcap، ترنسپورت KCP، هندشیک ping
- [x] port-forward، SOCKS5، session pool، iptables
- [x] چند upstream، health check، hot reload
- [x] Admin API، متریک، IPv6، نصب‌کننده
- [x] CLI برای reload/bench، احراز هویت admin، arm64
- [x] ترنسپورت QUIC با pinning دوطرفه گواهی
- [x] فریم‌بندی دیتاگرام UDP (رله با حفظ مرز)
- [x] حالت رِنج شفاف TCP «همهٔ پورت‌ها»
- [x] FEC و پنجره‌های قابل‌تنظیم KCP
- [x] reconnect خودترمیم upstream
- [x] داشبورد وب
- [x] پکیج‌های `.deb` / `.rpm`
- [x] مدیریت per-tunnel (لیست/ویرایش/حذف)
- [x] تست‌های یکپارچهٔ CI
- [ ] حالت رِنج شفاف UDP (TPROXY)
- [ ] burn-in دنیای واقعی و انتشار پایدار `1.0.0`

برای یادداشت‌های انتشار به [CHANGELOG.md](CHANGELOG.md) مراجعه کنید.

## مجوز

تحت **مجوز GNU General Public License v3.0 (GPL-3.0)** منتشر شده است. متن کامل در فایل [LICENSE](LICENSE) موجود است.
