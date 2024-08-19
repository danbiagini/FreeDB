sudo adduser --system --shell /bin/bash --home /home/incus incus
sudo adduser incus incus-admin
sudo su incus
incus admin init

# for debugging postgresql instance
sudo apt-get install postgresql-client-15
