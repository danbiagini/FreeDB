incus launch images:ubuntu/24.04/cloud cvat
incus exec cvat -- apt update

incus exec cvat -- sudo apt-get update
incus exec cvat -- sudo apt-get --no-install-recommends install -y \
  apt-transport-https \
  ca-certificates \
  curl \
  gnupg-agent \
  software-properties-common
incus exec cvat -- curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
incus exec cvat -- sudo add-apt-repository \
  "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) \
  stable"
incus exec cvat -- sudo apt-get update
incus exec cvat -- sudo apt-get --no-install-recommends install -y \
  docker-ce docker-ce-cli containerd.io docker-compose-plugin

incus exec cvat -- sudo groupadd docker
incus exec cvat -- sudo adduser --system --shell /bin/bash --home /home/cvat cvat
incus exec cvat -- sudo usermod -aG docker $USER

git clone https://github.com/cvat-ai/cvat
cd cvat
export CVAT_HOST=FQDN_or_YOUR-IP-ADDRESS

docker compose up -d

incus snapshot create cvat fresh-cvat-install

