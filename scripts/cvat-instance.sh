#setup disk and mount
# find the dev/<name> of the device to mount.  You can use ls -l /dev/disk/by-id/google-*
#sudo mkfs.ext4 -m 0 -E lazy_itable_init=0,lazy_journal_init=0,discard /dev/sdb
sudo mkdir -p /media/cvat-data
sudo mount -o discard,defaults /dev/sdb /media/cvat-data
sudo cp /etc/fstab /etc/fstab.backup-$(date +%s)
echo "UUID=$(sudo blkid /dev/sdb | cut -d '"' -f 2) /media/cvat-data ext4 discard,defaults 0 2" | sudo tee -a /etc/fstab > /dev/null

sudo apt-get --no-install-recommends install -y \
  apt-transport-https \
  ca-certificates \
  curl \
  gnupg-agent \
  software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository \
  "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) \
  stable"
sudo apt-get update
sudo apt-get --no-install-recommends install -y \
  docker-ce docker-ce-cli containerd.io docker-compose-plugin

sudo groupadd docker
sudo adduser --system --shell /bin/bash --home /home/cvat cvat
sudo usermod -aG docker $USER

git clone https://github.com/cvat-ai/cvat /media/cvat-data/cvat
cd /media/cvat-data/cvat
export CVAT_HOST=FQDN_or_YOUR-IP-ADDRESS

docker compose up -d

