#!/usr/bin/env bash
set -x

# Determine architecture
if [[ $(uname -s) == Darwin && $(uname -m) == amd64  ]]
then
	platform='Darwin_amd64'
elif [[ $(uname -s) == Darwin && $(uname -m) == x86_64  ]]
then
	platform='Darwin_arm64'
elif [[ $(uname -s) == Darwin && $(uname -m) == arm64  ]]
then
	platform='Darwin_arm64'
elif [[ $(uname -s) == Linux ]]
then
	platform='Linux_amd64'
else
	echo "No supported architecture found"
	exit 1
fi

jq_cmd=".assets[] | select(.name | endswith(\"${platform}.tar.gz\")).browser_download_url"
# Find latest binary release URL for this platform
url="$(curl -s https://api.github.com/repos/mbarley333/linkchecker/releases/latest | jq -r "${jq_cmd}")"
# Download the tarball
curl -OL ${url}
# Rename and copy to your blackjack folder
filename=$(basename $url)
#gunzip ${filename}
tar xvfz ${filename}
filename="linkchecker"
chmod +x ${filename}
LINKCHECKER_DIR=~/linkchecker/$platform
mkdir -p $LINKCHECKER_DIR
mv $filename ${LINKCHECKER_DIR}/${filename%_${platform}}