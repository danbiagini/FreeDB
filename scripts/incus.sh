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

# TO setup incus to be able to launch containers from gcloud artifact registry


# First, let's create a temporary variable with the base64 encoded credentials
AUTH_STRING=$(echo -n "_json_key:$(cat ~/key.json)" | base64 -w0)

# Now create the auth.json file
sudo -u incuscat > ~/.config/containers/auth.json << EOF
{
  "auths": {
    "us-central1-docker.pkg.dev": {
      "auth": "${AUTH_STRING}"
    }
  }
}
EOF

echo "Setting incus environment variable for use by skopeo"
# Custom environment for freeDB w/ OCI container support 
echo "# Setup for incus w/ OCI container support" >> /etc/default/incus
echo "XDG_RUNTIME_DIR=/home/incus/.config" >> /etc/default/incus

incus remote add gcr https://us-central1-docker.pkg.dev
# incus launch gcr:PROJECT-ID/REPOSITORY/IMAGE

# Setup DNS for incus
sudo cp config/incus-dns.service /etc/systemd/system/incus-dns-incusbr0.service
sudo systemctl enable incus-dns-incusbr0.service
sudo systemctl start incus-dns-incusbr0.service
