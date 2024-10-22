sudo apt-get install -yq postgresql-client-15
sudo adduser --system --shell /bin/bash --home /home/incus incus
sudo adduser incus incus-admin
sudo su - incus
incus admin init
