#!/usr/bin/env bash

# Original script created by the dep dependency manager for Golang project
# 
# This install script is intended to download and install the latest available
# release of the linkchecker.
#
# It attempts to identify the current platform and an error will be thrown if
# the platform is not supported.
#
#
# 
# You can install using this script:
# $ curl https://raw.githubusercontent.com/mbarley333/linkchecker/main/install.sh | sh
# ​
set -e

RELEASES_API="https://api.github.com/repos/mbarley333/linkchecker/releases/latest"





initArch() {
    ARCH=$(uname -m)
    if [ -n "$DEP_ARCH" ]; then
        echo "Using DEP_ARCH"
        ARCH="$DEP_ARCH"
    fi
    case $ARCH in
        amd64) ARCH="amd64";;
        x86_64) ARCH="amd64";;
        i386) ARCH="386";;
        ppc64) ARCH="ppc64";;
        ppc64le) ARCH="ppc64le";;
        s390x) ARCH="s390x";;
        armv6*) ARCH="arm";;
        armv7*) ARCH="arm";;
        aarch64) ARCH="arm64";;
		arm64) ARCH="arm64";;
        *) echo "Architecture ${ARCH} is not supported by this installation script"; exit 1;;
    esac
    echo "ARCH = $ARCH"
}
initOS() {
    OS=$(uname | tr '[:upper:]' '[:lower:]')
    OS_CYGWIN=0
    if [ -n "$DEP_OS" ]; then
        echo "Using DEP_OS"
        OS="$DEP_OS"
    fi
    case "$OS" in
        darwin) OS='darwin';;
        linux) OS='linux';;
        freebsd) OS='freebsd';;
        mingw*) OS='windows';;
        msys*) OS='windows';;
	cygwin*)
	    OS='windows'
	    OS_CYGWIN=1
	    ;;
        *) echo "OS ${OS} is not supported by this installation script"; exit 1;;
    esac
    echo "OS = $OS"
}

# identify platform based on uname output
initArch
initOS

echo "Will install into current directory"

# assemble expected release artifact name
if [ "${OS}" != "linux" ] && { [ "${ARCH}" = "ppc64" ] || [ "${ARCH}" = "ppc64le" ];}; then
    # ppc64 and ppc64le are only supported on Linux.
    echo "${OS}-${ARCH} is not supported by this instalation script"
fi


# build binary download url
jq_cmd=".assets[] | select(.name | endswith(\"${OS}_${ARCH}.tar.gz\")).browser_download_url"
BINARY_URL="$(curl -s $RELEASES_API | jq -r "${jq_cmd}")"

#download binary
filename=$(basename $BINARY_URL)
curl -OL ${BINARY_URL}
tar xvfz ${filename}
filename="linkchecker"
chmod +x ${filename}

echo "Executable downloaded into current directory"


