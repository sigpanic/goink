#!/bin/bash
set -euo pipefail

OS=$(uname -s)
RUNTIME_DIR="${RUNTIME_DIR:-build/runtime}"
ONNX_VERSION="1.26.0"
BGE_REPO="https://huggingface.co/Xenova/bge-small-zh-v1.5/resolve/main"
BGE_MIRROR="https://hf-mirror.com/Xenova/bge-small-zh-v1.5/resolve/main"
MODEL_DIR="${RUNTIME_DIR}/models"

download_bge_model() {
    mkdir -p "$MODEL_DIR"
    echo "下载 BGE 嵌入模型 (bge-small-zh-v1.5, int8)..."

    # vocab.txt
    if ! curl -fsSL --retry 2 --retry-delay 10 --connect-timeout 30 -o "$MODEL_DIR/vocab.txt" "$BGE_REPO/vocab.txt"; then
        echo "HF 直连失败，尝试镜像..."
        curl -fsSL --retry 2 --retry-delay 5 --connect-timeout 30 -o "$MODEL_DIR/vocab.txt" "$BGE_MIRROR/vocab.txt"
    fi

    # model.onnx (int8 quantized)
    if ! curl -fsSL --retry 2 --retry-delay 10 --connect-timeout 30 -o "$MODEL_DIR/model.onnx" "$BGE_REPO/onnx/model_int8.onnx"; then
        echo "HF 直连失败，尝试镜像..."
        curl -fsSL --retry 2 --retry-delay 5 --connect-timeout 30 -o "$MODEL_DIR/model.onnx" "$BGE_MIRROR/onnx/model_int8.onnx"
    fi

    # 校验
    if file "$MODEL_DIR/model.onnx" | grep -qi "html"; then
        echo "错误: 下载的模型文件是 HTML 页面"
        head -5 "$MODEL_DIR/model.onnx"
        exit 1
    fi
    echo "BGE 模型 → $MODEL_DIR/"
    ls -la "$MODEL_DIR/"
}

download_onnx() {
    local os_tag="$1"
    local file="$2"
    local url="https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/${file}"

    mkdir -p "$RUNTIME_DIR"
    echo "下载 ONNX Runtime ${ONNX_VERSION} (${os_tag})..."

    curl -fsSL --retry 3 --connect-timeout 30 -o "/tmp/${file}" "$url"

    # 校验下载内容不是 HTML 错误页
    if file "/tmp/${file}" | grep -qi "html"; then
        echo "错误: 下载的内容是 HTML 页面，非有效压缩包"
        head -5 "/tmp/${file}"
        exit 1
    fi

    echo "解压..."
    rm -rf /tmp/onnx-extract
    if [[ "$file" == *.zip ]]; then
        unzip -qo "/tmp/${file}" -d /tmp/onnx-extract
    else
        mkdir -p /tmp/onnx-extract
        tar -xzf "/tmp/${file}" -C /tmp/onnx-extract
    fi

    # ONNX Runtime 包结构固定: <name>/lib/ 下有所有库文件和 .pc
    local lib_dir
    lib_dir=$(find /tmp/onnx-extract -type d -name "lib" | head -1)
    if [ -z "$lib_dir" ]; then
        echo "错误: 未找到 lib 目录，包结构如下:"
        find /tmp/onnx-extract -type f | head -20
        exit 1
    fi
    # 复制库文件（Win 无 lib 前缀：onnxruntime.dll；Linux/macOS：libonnxruntime.so/.dylib）
    # 排除 .pdb（调试符号）和 .lib（导入库），运行时不需要
    find "$lib_dir" -maxdepth 1 -type f ! -name "*.pdb" ! -name "*.lib" \( -name "*onnxruntime*" -o -name "*.pc" \) -exec cp {} "$RUNTIME_DIR/" \;
    # 创建不带版本号的 symlink（Linux: libonnxruntime.so，macOS: libonnxruntime.dylib）
    local symlinked=0
    for f in "$RUNTIME_DIR"/libonnxruntime.so.*; do
        [ -f "$f" ] && ln -sf "$(basename "$f")" "$RUNTIME_DIR/libonnxruntime.so" && symlinked=1 && break
    done
    for f in "$RUNTIME_DIR"/libonnxruntime.*.dylib; do
        [ -f "$f" ] && ln -sf "$(basename "$f")" "$RUNTIME_DIR/libonnxruntime.dylib" && symlinked=1 && break
    done
    if [ "$symlinked" -eq 0 ] && [[ "$file" != *.zip ]]; then
        echo "错误: 未找到带版本号的 ONNX 库文件，无法创建 symlink"
        ls -la "$RUNTIME_DIR/"
        exit 1
    fi

    rm -rf /tmp/onnx-extract "/tmp/${file}"
    echo "ONNX Runtime → $RUNTIME_DIR/"
    ls -la "$RUNTIME_DIR/"
}

# download_vc_runtime 从官方 VC++ Redistributable 提取 vcruntime140.dll 等到 $RUNTIME_DIR。
# ONNX Runtime（onnxruntime.dll）动态链接 VC++ Runtime（vcruntime140.dll、
# vcruntime140_1.dll、msvcp140.dll）。放到 onnxruntime.dll 同目录，利用
# Windows DLL 搜索优先级（应用目录 > System32 > PATH）避免用户系统 PATH 冲突
# 或版本不匹配导致的 DllMain 初始化失败。
# 仅 Windows 调用；Linux/macOS 不需要。
download_vc_runtime() {
    echo "下载 VC++ Redistributable (x64)..."
    local vc_redist="/tmp/vc_redist.x64.exe"
    if ! curl -fsSL --retry 3 --connect-timeout 30 -o "$vc_redist" "https://aka.ms/vs/17/release/vc_redist.x64.exe"; then
        echo "警告: 下载 VC++ Redist 失败，跳过（假设系统已安装）"
        return 0
    fi

    if ! command -v 7z >/dev/null 2>&1; then
        echo "警告: 未找到 7z，跳过 VC++ Runtime 提取（CI 已预装，本地需自行安装 7-Zip）"
        rm -f "$vc_redist"
        return 0
    fi

    local extract_dir="/tmp/vc-redist-extract"
    rm -rf "$extract_dir"
    mkdir -p "$extract_dir"

    # 第一步：解压 SFX exe
    if ! 7z x -o"$extract_dir" "$vc_redist" -y >/dev/null 2>&1; then
        echo "警告: 解压 VC Redist exe 失败，跳过"
        rm -rf "$extract_dir" "$vc_redist"
        return 0
    fi

    # 第二步：递归查找已直接暴露的 DLL（部分 redist 结构）
    local found=0
    while IFS= read -r -d '' dll; do
        cp -f "$dll" "$RUNTIME_DIR/"
        found=1
        echo "  $(basename "$dll") → $RUNTIME_DIR/"
    done < <(find "$extract_dir" \( -name "vcruntime*.dll" -o -name "msvcp*.dll" \) -print0 2>/dev/null)

    # 第三步：若未找到，VC 14+ redist 把 DLL 封在 MSI 内，逐个解压 MSI 再查找
    if [ "$found" -eq 0 ]; then
        local msi_extract="$extract_dir/_msi"
        mkdir -p "$msi_extract"
        while IFS= read -r -d '' msi; do
            7z x -o"$msi_extract" "$msi" -y >/dev/null 2>&1 || true
        done < <(find "$extract_dir" -name "*.msi" -print0 2>/dev/null)

        while IFS= read -r -d '' dll; do
            cp -f "$dll" "$RUNTIME_DIR/"
            found=1
            echo "  $(basename "$dll") → $RUNTIME_DIR/"
        done < <(find "$msi_extract" \( -name "vcruntime*.dll" -o -name "msvcp*.dll" \) -print0 2>/dev/null)
    fi

    rm -rf "$extract_dir" "$vc_redist"

    if [ "$found" -eq 0 ]; then
        echo "警告: 未能从 VC Redist 提取 DLL，跳过"
    else
        echo "VC++ Runtime DLL → $RUNTIME_DIR/"
    fi
}

case "${OS}" in
    MINGW*|MSYS*|CYGWIN*)
        download_onnx "win-x64" "onnxruntime-win-x64-${ONNX_VERSION}.zip"
        download_vc_runtime
        download_bge_model
        ;;
    Linux)
        download_onnx "linux-x64" "onnxruntime-linux-x64-${ONNX_VERSION}.tgz"
        download_bge_model
        ;;
    Darwin)
        download_onnx "osx-arm64" "onnxruntime-osx-arm64-${ONNX_VERSION}.tgz"
        download_bge_model
        ;;
    *)
        echo "不支持的操作系统: $OS"
        exit 1
        ;;
esac
