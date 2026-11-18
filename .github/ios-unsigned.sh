#!/usr/bin/env bash
# 构建未签名的 iOS IPA，交给 AltStore / SideStore 用用户自己的 Apple ID 签名。
# MyGo 只会用描述文件签设备包，项目没有 Apple 开发者账号：这里造一个只在 CI 里用的
# 自签名身份和描述文件让它签名打包，打包后再去掉签名和描述文件。
# 用法：在 app/ 里运行 ../.github/ios-unsigned.sh <输出的 .ipa 路径>
set -euo pipefail

mkdir -p "$(dirname "$1")"
out="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
id="$(jq -r .identifier mygo.json)"
team=CIPLACEHLD
work="$RUNNER_TEMP/ios-sign"
rm -rf "$work"
mkdir -p "$work"

# 自签名的代码签名证书。用系统的 openssl（LibreSSL）：它导出的 p12 security 才认
ossl=/usr/bin/openssl
cat > "$work/cert.cnf" <<EOF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = Bili Daily CI
OU = $team
[ext]
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
basicConstraints = critical, CA:false
EOF
$ossl req -x509 -newkey rsa:2048 -nodes -days 7 -config "$work/cert.cnf" -keyout "$work/key.pem" -out "$work/cert.pem" 2>/dev/null
$ossl pkcs12 -export -inkey "$work/key.pem" -in "$work/cert.pem" -out "$work/id.p12" -passout pass:ci

keychain="$work/ci.keychain-db"
security create-keychain -p ci "$keychain"
security set-keychain-settings "$keychain"
security unlock-keychain -p ci "$keychain"
# shellcheck disable=SC2046
security list-keychains -d user -s "$keychain" $(security list-keychains -d user | tr -d '"')
security import "$work/id.p12" -k "$keychain" -P ci -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k ci "$keychain" >/dev/null
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$work/cert.pem"
security find-identity -v -p codesigning | grep -q "Bili Daily CI" || { echo "::error::自签名身份没有出现在钥匙串里"; security find-identity -v -p codesigning; exit 1; }

# 描述文件：app-store 类型（没有设备列表、没有 get-task-allow），证书是上面那张
uuid="$(uuidgen)"
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
expires="$(date -u -v+7d +%Y-%m-%dT%H:%M:%SZ)"
cert="$($ossl x509 -in "$work/cert.pem" -outform DER | base64 | tr -d '\n')"
cat > "$work/profile.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>AppIDName</key><string>Bili Daily CI</string>
  <key>ApplicationIdentifierPrefix</key><array><string>$team</string></array>
  <key>CreationDate</key><date>$now</date>
  <key>DeveloperCertificates</key><array><data>$cert</data></array>
  <key>Entitlements</key>
  <dict>
    <key>application-identifier</key><string>$team.$id</string>
    <key>com.apple.developer.team-identifier</key><string>$team</string>
  </dict>
  <key>ExpirationDate</key><date>$expires</date>
  <key>Name</key><string>Bili Daily CI</string>
  <key>TeamIdentifier</key><array><string>$team</string></array>
  <key>TeamName</key><string>Bili Daily CI</string>
  <key>TimeToLive</key><integer>7</integer>
  <key>UUID</key><string>$uuid</string>
  <key>Version</key><integer>1</integer>
</dict>
</plist>
EOF
profiles="$HOME/Library/MobileDevice/Provisioning Profiles"
mkdir -p "$profiles"
$ossl smime -sign -nodetach -binary -outform DER -signer "$work/cert.pem" -inkey "$work/key.pem" \
  -in "$work/profile.plist" -out "$profiles/$uuid.mobileprovision"
security cms -D -i "$profiles/$uuid.mobileprovision" >/dev/null || { echo "::error::security 解不开自造的描述文件"; exit 1; }

go tool mygo build -platform ios
ipa="$(pwd)/$(ls build/ios-arm64/*.ipa)"

# 去掉签名和描述文件
rm -rf "$work/ipa"
mkdir -p "$work/ipa"
(cd "$work/ipa" && unzip -q "$ipa")
app="$(ls -d "$work"/ipa/Payload/*.app)"
rm -f "$app/embedded.mobileprovision"
if [ -d "$app/Frameworks" ]; then
  find "$app/Frameworks" -maxdepth 1 \( -name '*.framework' -o -name '*.dylib' \) -exec codesign --remove-signature {} \;
fi
codesign --remove-signature "$app"
find "$app" -name _CodeSignature -type d -prune -exec rm -rf {} +
if codesign -dv "$app" 2>/dev/null; then
  echo "::error::IPA 里的 App 仍然带着签名"
  exit 1
fi
rm -f "$out"
(cd "$work/ipa" && zip -qry "$out" Payload)
echo "$out"
