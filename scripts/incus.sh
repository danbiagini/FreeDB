sudo apt-get install -yq postgresql-client-15
sudo adduser --system --shell /bin/bash --home /home/incus incus
sudo adduser incus incus-admin
sudo su - incus
incus admin init

# https://openzfs.github.io/openzfs-docs/Getting%20Started/Debian/
# https://blog.simos.info/how-to-install-and-set-up-incus-on-a-cloud-server/
# https://www.cyberciti.biz/faq/installing-zfs-on-debian-12-bookworm-linux-apt-get/

incus storage create pd-standard zfs source=/dev/sdb
incus profile copy default v1
incus profile edit v1 # switch to the new storage pool