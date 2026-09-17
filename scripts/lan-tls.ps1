# 局域网 HTTPS 助手：签一张带本机 IP 的证书，交给 deploy/Caddyfile 做 TLS 终止。
#
#   .\scripts\lan-tls.ps1                      # 自动识别局域网 IP，优先用 mkcert
#   .\scripts\lan-tls.ps1 -Ip 192.168.1.23     # 明确指定地址
#   .\scripts\lan-tls.ps1 -Dns nas.lan -Force  # 追加域名 / 重新签发
#
# 证书默认写到 data/tls（.gitignore 已忽略），文件名与 deploy/Caddyfile 的默认值
# 一致，所以签完只要改 Caddyfile 第一行的地址就能直接跑。
#
# 为什么不建议「随便一张自签证书」：浏览器只在安全上下文里提供 crypto.subtle，
# 而没被设备信任的证书即使点了「继续访问」，那个 origin 仍然不算安全上下文，
# 登录照旧报「无法加密口令」。证书必须真正装进设备的信任库，见 docs/deployment.md 3.4。

[CmdletBinding()]
param(
    [string]$Ip,
    [string[]]$Dns = @(),
    [string]$OutDir,
    [switch]$Force
)

# 原生工具往 stderr 写警告是常态，不要让它变成终止错误；失败一律看 $LASTEXITCODE。
$ErrorActionPreference = 'Continue'

$repo = Split-Path -Parent $PSScriptRoot
if (-not $OutDir) { $OutDir = Join-Path $repo 'data\tls' }
elseif (-not [System.IO.Path]::IsPathRooted($OutDir)) { $OutDir = Join-Path $repo $OutDir }

function Get-LanAddress {
    # 用 UDP「连接」一个外网地址，问出默认路由那张网卡的本地地址。不实际发包、
    # 不需要管理员权限，比枚举网卡更贴近「别的设备会怎么找到我」。
    try {
        $socket = [System.Net.Sockets.UdpClient]::new()
        try {
            $socket.Connect('8.8.8.8', 65530)
            $address = $socket.Client.LocalEndPoint.Address
            if ($address -and -not [System.Net.IPAddress]::IsLoopback($address)) { return $address.ToString() }
        } finally { $socket.Dispose() }
    } catch { }

    try {
        $resolved = [System.Net.Dns]::GetHostAddresses([System.Net.Dns]::GetHostName()) |
            Where-Object { $_.AddressFamily -eq 'InterNetwork' -and -not [System.Net.IPAddress]::IsLoopback($_) } |
            Select-Object -First 1
        if ($resolved) { return $resolved.ToString() }
    } catch { }

    return ''
}

# 自动识别出来的地址必须落在内网段。VPN、虚拟网卡、网关很容易被当成默认路由，
# 抓到那种地址会签出一张对方根本用不上的证书，所以宁可直接拒绝、要求显式指定。
function Test-PrivateAddress {
    param([string]$Address)
    $parts = @($Address.Split('.') | ForEach-Object { [int]$_ })
    if ($parts[0] -eq 10) { return $true }
    if ($parts[0] -eq 172 -and $parts[1] -ge 16 -and $parts[1] -le 31) { return $true }
    if ($parts[0] -eq 192 -and $parts[1] -eq 168) { return $true }
    if ($parts[0] -eq 169 -and $parts[1] -eq 254) { return $true }
    if ($parts[0] -eq 100 -and $parts[1] -ge 64 -and $parts[1] -le 127) { return $true }
    return $false
}

if (-not $Ip) {
    $Ip = Get-LanAddress
    if (-not $Ip) {
        Write-Host '没能自动识别局域网 IP，请显式指定，例如：.\scripts\lan-tls.ps1 -Ip 192.168.1.23' -ForegroundColor Red
        exit 1
    }
    if (-not (Test-PrivateAddress $Ip)) {
        Write-Host "自动识别到的是 $Ip，不是内网地址（十有八九抓到了 VPN 或虚拟网卡）。" -ForegroundColor Red
        $candidates = @()
        try {
            $candidates = [System.Net.Dns]::GetHostAddresses([System.Net.Dns]::GetHostName()) |
                Where-Object { $_.AddressFamily -eq 'InterNetwork' } |
                ForEach-Object { $_.ToString() }
        } catch { }
        if ($candidates.Count -gt 0) { Write-Host "本机证书上能写的其它 IPv4 地址：$($candidates -join '、')" }
        Write-Host '请显式指定要签进证书的地址，例如：.\scripts\lan-tls.ps1 -Ip 192.168.1.23' -ForegroundColor Red
        exit 1
    }
}
if ($Ip -notmatch '^\d{1,3}(\.\d{1,3}){3}$') {
    Write-Host "-Ip 需要是 IPv4 地址，收到的是 '$Ip'" -ForegroundColor Red
    exit 1
}

$hostName = [System.Net.Dns]::GetHostName()
$certPath = Join-Path $OutDir 'server.pem'
$keyPath = Join-Path $OutDir 'server-key.pem'
$caPath = Join-Path $OutDir 'ca.pem'
$site = "https://$Ip"

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Write-Host ''
Write-Host "局域网地址 : $Ip"
Write-Host "主机名     : $hostName"
Write-Host "输出目录   : $OutDir"
Write-Host ''

if ((Test-Path $certPath) -and -not $Force) {
    Write-Host "证书已存在（要重新签发请加 -Force）：$certPath" -ForegroundColor Yellow
    exit 0
}

$caHint = ''
$mkcert = Get-Command mkcert -ErrorAction SilentlyContinue

if ($mkcert) {
    Write-Host '用 mkcert 签发（会把本地 CA 装进本机信任库）...' -ForegroundColor Cyan
    & $mkcert.Source -install | Out-Null
    & $mkcert.Source -cert-file $certPath -key-file $keyPath $Ip localhost 127.0.0.1 $hostName @Dns
    if ($LASTEXITCODE -ne 0) {
        Write-Host "mkcert 签发失败（exit $LASTEXITCODE）" -ForegroundColor Red
        exit 1
    }
    $caPath = (& $mkcert.Source -CAROOT).Trim() + [System.IO.Path]::DirectorySeparatorChar + 'rootCA.pem'
    $caHint = 'mkcert 的根证书（装到手机 / 其它电脑上）'
} else {
    $openssl = Get-Command openssl -ErrorAction SilentlyContinue
    if (-not $openssl) {
        Write-Host '需要 mkcert 或 openssl 其中之一：' -ForegroundColor Red
        Write-Host '  winget install FiloSottile.mkcert      # 推荐，会自动装好本机信任库'
        Write-Host '  winget install ShiningLight.OpenSSL.Light'
        exit 1
    }

    Write-Host 'mkcert 不在，改用 openssl 自己签一套 CA + 服务端证书...' -ForegroundColor Cyan
    Write-Host '（这条路需要你手动把 ca.pem 装到每台访问设备上）' -ForegroundColor Yellow

    # msys2 / Git-Bash 的 openssl 会把 `-subj /CN=...` 这类参数当成路径改写，
    # 所以 DN 与 SAN 一律走配置文件，不用 -subj。
    $sanLines = @()
    $ipList = @($Ip, '127.0.0.1')
    for ($i = 0; $i -lt $ipList.Count; $i++) { $sanLines += "IP.$($i + 1) = $($ipList[$i])" }
    $dnsList = @('localhost', $hostName) + $Dns | Select-Object -Unique
    for ($i = 0; $i -lt $dnsList.Count; $i++) { $sanLines += "DNS.$($i + 1) = $($dnsList[$i])" }

    $caConf = Join-Path $OutDir 'ca.cnf'
    $srvConf = Join-Path $OutDir 'server.cnf'
    @"
[req]
distinguished_name = req_dn
prompt = no
x509_extensions = v3_ca

[req_dn]
CN = Nagisa LAN CA

[v3_ca]
basicConstraints = critical, CA:TRUE
keyUsage = critical, keyCertSign, cRLSign
"@ | Set-Content -Path $caConf -Encoding ascii

    @"
[req]
distinguished_name = req_dn
prompt = no

[req_dn]
CN = $Ip

[ext]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
$($sanLines -join "`n")
"@ | Set-Content -Path $srvConf -Encoding ascii

    $caKey = Join-Path $OutDir 'ca-key.pem'
    $csr = Join-Path $OutDir 'server.csr'

    & $openssl.Source req -x509 -newkey rsa:3072 -sha256 -days 3650 -nodes `
        -keyout $caKey -out $caPath -config $caConf 2>$null
    if ($LASTEXITCODE -ne 0) { Write-Host '生成 CA 失败' -ForegroundColor Red; exit 1 }

    & $openssl.Source req -new -newkey rsa:3072 -sha256 -nodes `
        -keyout $keyPath -out $csr -config $srvConf 2>$null
    if ($LASTEXITCODE -ne 0) { Write-Host '生成服务端 CSR 失败' -ForegroundColor Red; exit 1 }

    # 825 天：iOS / macOS 对叶子证书的上限，别签更长。
    & $openssl.Source x509 -req -in $csr -CA $caPath -CAkey $caKey -CAcreateserial `
        -out $certPath -days 825 -sha256 -extfile $srvConf -extensions ext 2>$null
    if ($LASTEXITCODE -ne 0) { Write-Host '签发服务端证书失败' -ForegroundColor Red; exit 1 }

    Remove-Item $csr -ErrorAction SilentlyContinue
    $caHint = 'ca.pem（自建 CA，装到手机 / 其它电脑上）'
}

Write-Host ''
Write-Host '证书就绪：' -ForegroundColor Green
Write-Host "  证书 $certPath"
Write-Host "  私钥 $keyPath"
Write-Host "  CA   $caPath"
Write-Host ''
Write-Host '接下来：' -ForegroundColor Cyan
Write-Host "  1. 把 deploy/Caddyfile 第一行改成：$site {"
Write-Host '  2. 在仓库根目录启动代理：caddy run --config deploy/Caddyfile'
Write-Host "  3. 浏览器打开 $site"
Write-Host "  4. 每台要访问的设备都要信任上面那个 $caHint"
Write-Host '     Android：设置 → 安全 → 加密与凭据 → 安装证书 → CA 证书'
Write-Host '     iOS    ：把 ca.pem 发过去点开 → 设置 → 通用 → 关于本机 → 证书信任设置 → 打开完全信任'
Write-Host ''
Write-Host '同时把这些配置改成设备实际访问的地址，否则分享链接仍会指回 127.0.0.1：' -ForegroundColor Cyan
Write-Host "  web.public_base_url: $site"
Write-Host '  web.cors_origins   : 同源部署留空；用 Vite 调试时加前端源站'
Write-Host '  data.object_storage.public_endpoint: 浏览器可达的对象存储地址'


