sudo apt-get update
sudo apt-get install -yq postgresql-client-16 git curl

#setup disk and mount
# find the dev/<name> of the device to mount.  You can use ls -l /dev/disk/by-id/google-*

# this wipes the disk so leave it commented out for now
# sudo mkfs.ext4 -m 0 -E lazy_itable_init=0,lazy_journal_init=0,discard /dev/disk/by-id/google-cvat-data-1 
sudo mkdir -p /mnt/cvat-data
sudo mount -o discard,defaults /dev/disk/by-id/google-cvat-data-1 /mnt/cvat-data
sudo cp /etc/fstab /etc/fstab.backup-$(date +%s)
echo "/dev/disk/by-id/google-cvat-data-1 /mnt/cvat-data ext4 discard,defaults,nofail 0 2" | sudo tee -a /etc/fstab > /dev/null

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

# w3m shell browser for debugging 
sudo apt-get install -y \
  w3m w3m-img

sudo groupadd docker
sudo adduser --system --group --shell /bin/bash --home /home/cvat cvat
sudo usermod -aG docker cvat
sudo -u cvat cp /etc/skel/.* /home/cvat/

git clone https://github.com/cvat-ai/cvat /mnt/cvat-data/cvat
cd /mnt/cvat-data/cvat

cat <<EOF > .env
export CVAT_HOST=${CVAT_DOMAIN}
export CVAT_POSTGRES_HOST=freedb.${ZONE}.c.${PROJECT}.internal
export CVAT_POSTGRES_DBNAME=cvat
export CVAT_POSTGRES_USER=cvat
export CVAT_POSTGRES_PASSWORD=${CVAT_DB_PASSWORD}

EOF




