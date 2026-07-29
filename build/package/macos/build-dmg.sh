#!/bin/bash
set -euo pipefail

VERSION="${1:-dev}"
APP_NAME="goink"
# Wails on macOS outputs a .app bundle at build/bin/goink.app
APP_BUNDLE="build/bin/${APP_NAME}.app"
RUNTIME_DIR="build/runtime"

echo "使用 Wails 生成的 .app: $APP_BUNDLE"

# 将运行时注入已有 .app bundle
mkdir -p "$APP_BUNDLE/Contents/Resources/runtime/git"
cp "$RUNTIME_DIR/git/git" "$APP_BUNDLE/Contents/Resources/runtime/git/"
find "$RUNTIME_DIR" -maxdepth 1 \( -name "libonnxruntime*" -o -name "*.pc" \) -exec cp -R {} "$APP_BUNDLE/Contents/Resources/runtime/" \;
# 模型文件
if [ -d "$RUNTIME_DIR/models" ]; then
	cp -r "$RUNTIME_DIR/models" "$APP_BUNDLE/Contents/Resources/runtime/"
fi
# 确保可执行
chmod +x "$APP_BUNDLE/Contents/Resources/runtime/git"
chmod +x "$APP_BUNDLE/Contents/Resources/runtime/libonnxruntime"* 2>/dev/null || true

# Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>Goink</string>
    <key>CFBundleDisplayName</key>
    <string>Goink</string>
    <key>CFBundleIdentifier</key>
    <string>com.goink.app</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleExecutable</key>
    <string>goink</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

# build-dmg.sh 重写了 Info.plist，破坏了 Wails 在 build 阶段做的 ad-hoc 签名 seal
# 按 Apple 推荐做法（不用 deprecated 的 --deep），只重签 .app bundle 自身
# 嵌套的 git（Apple 系统签名）和 libonnxruntime.dylib（微软签名）保留各自原签名
codesign --force --sign - "$APP_BUNDLE"

# 图标占位
[ -f build/package/macos/goink.icns ] && cp build/package/macos/goink.icns "$APP_BUNDLE/Contents/Resources/"

# 生成 DMG
mkdir -p build/dist
hdiutil create -volname "Goink" -srcfolder "$APP_BUNDLE" -ov -format UDZO \
    "build/dist/goink-v${VERSION}-macos-arm64.dmg"

echo "DMG → build/dist/goink-v${VERSION}-macos-arm64.dmg"
