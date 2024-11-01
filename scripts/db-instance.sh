sudo -u incus incus launch images:debian/12/cloud db1
sudo -u incus incus exec db1 -- apt install -yq postgresql curl cron

# https://wiki.debian.org/PostgreSql
sudo -u incus incus exec db1 -- sudo adduser --system --shell /bin/bash --home /home/sportsoil sportsoil
sudo -u incus incus exec db1 -- sudo -u postgres createuser -d sportsoil
sudo -u incus incus exec db1 -- sudo -u sportsoil createdb -O sportsoil sportsoil

# add pg_hba entry for client and accept tcp connections
sudo -u incus incus exec db1 -- sudo -u postgres sed -r -i.BAK "/#listen_addresses/a\listen_addresses = '*'" /etc/postgresql/15/main/postgresql.conf

# add incus network forward to send postgres traffic from host to instance.  this will only
# list on the internal address for database connections
sudo -u incus incus network forward create incusbr0 

sudo -u incus incus network forward create incusbr0 10.0.1.14
sudo -u incus incus network forward port add incusbr0 10.0.1.14 tcp 5432 10.0.0.224

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 

# setup nightly pg_dump cron job
sudo -u incus incus exec db1 -- sudo -u postgres curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-cli-linux-x86_64.tar.gz
sudo -u incus incus exec db1 -- tar -xf google-cloud-cli-linux-x86_64.tar.gz

# setup the postgres user with bash and PATH after the gcloud install
sudo -u incus incus exec db1 -- cp /etc/skel/.* /var/lib/postgresql/

# TODO: fix this, it will be interactive
#sudo -u incus incus exec db1 -- gcloud init