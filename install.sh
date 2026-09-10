#!/bin/sh
set -eu

repository="ArcheMind/agentx"
install_dir="${AX_INSTALL_DIR:-${HOME}/.local/bin}"
requested_version="${AX_VERSION:-latest}"

case "$(uname -s)" in
	Darwin) target_os="Darwin" ;;
	Linux) target_os="Linux" ;;
	*) echo "ax: unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) target_arch="x86_64" ;;
	arm64|aarch64) target_arch="arm64" ;;
	*) echo "ax: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

archive="agentx_${target_os}_${target_arch}.tar.gz"
if [ "$requested_version" = "latest" ]; then
	base_url="https://github.com/${repository}/releases/latest/download"
else
	case "$requested_version" in v*) tag="$requested_version" ;; *) tag="v${requested_version}" ;; esac
	base_url="https://github.com/${repository}/releases/download/${tag}"
fi

temporary_dir="$(mktemp -d)"
trap 'rm -rf "$temporary_dir"' EXIT INT TERM

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -LsSf "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		echo "ax: curl or wget is required" >&2
		exit 1
	fi
}

download "${base_url}/${archive}" "${temporary_dir}/${archive}"
download "${base_url}/checksums.txt" "${temporary_dir}/checksums.txt"

expected="$(awk -v file="$archive" '$2 == file { print $1 }' "${temporary_dir}/checksums.txt")"
if [ -z "$expected" ]; then
	echo "ax: checksum for ${archive} was not found" >&2
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "${temporary_dir}/${archive}" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "${temporary_dir}/${archive}" | awk '{ print $1 }')"
else
	echo "ax: sha256sum or shasum is required to verify the download" >&2
	exit 1
fi

if [ "$actual" != "$expected" ]; then
	echo "ax: checksum verification failed for ${archive}" >&2
	exit 1
fi

tar -xzf "${temporary_dir}/${archive}" -C "$temporary_dir" ax
mkdir -p "$install_dir"
install -m 0755 "${temporary_dir}/ax" "${install_dir}/ax"

echo "installed ax to ${install_dir}/ax"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*) echo "add ${install_dir} to PATH to run ax" ;;
esac
