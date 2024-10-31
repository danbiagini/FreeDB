# needed for zabbly package install (a more recent version for debian 12).
# https://github.com/zabbly/incus
sudo curl -fsSL https://pkgs.zabbly.com/key.asc -o /etc/apt/keyrings/zabbly.asc
sudo sh -c 'cat <<EOF > /etc/apt/sources.list.d/zabbly-incus-stable.sources
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/incus/stable
Suites: $(. /etc/os-release && echo ${VERSION_CODENAME})
Components: main
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/zabbly.asc

EOF'

sudo apt-get update; sudo apt-get install -yq incus
sudo apt-get install -yq postgresql-client-15

sudo sed -r -i'.BAK' 's/^Components(.*)$/Components\1 contrib/g' /etc/apt/sources.list.d/debian.sources
# sudo apt install zfsutils-linux
sudo deluser incus
sudo adduser --system --shell /bin/bash --home /home/incus incus
sudo usermod -aG incus-admin incus 
sudo usermod -aG sudo incus
sudo -u incus cp /etc/skel/.* /home/incus/

# this command must be interactive because the zfs install has a license warning
# apt install linux-headers-cloud-amd64 zfsutils-linux zfs-dkms zfs-zed
#incus admin init

# https://openzfs.github.io/openzfs-docs/Getting%20Started/Debian/
# https://blog.simos.info/how-to-install-and-set-up-incus-on-a-cloud-server/
# https://www.cyberciti.biz/faq/installing-zfs-on-debian-12-bookworm-linux-apt-get/

# TODO: need to determine the correct device for the disk

#incus storage create pd-standard zfs source=/dev/sdb
#incus profile copy default v1
#incus profile edit v1 # switch to the new storage pool