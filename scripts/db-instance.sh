sudo -u incus incus launch images:debian/12/cloud db1
sudo -u incus incus exec db1 -- apt install -yq postgresql curl cron

# setup the postgres user with dot files
sudo -u incus incus exec db1 -- sudo -u postgres cp /etc/skel/.* /var/lib/postgresql/

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

sudo -u incus incus exec db1 -- sudo -u postgres cp /etc/postgresql/15/main/pg_hba.conf /etc/postgresql/15/main/pg_hba.conf.bak
sudo -u incus incus exec db1 -- sudo -u postgres sh -c 'cat <<EOF >> /etc/postgresql/15/main/pg_hba.conf

## FreeDB host system and container connections
host    all             all             10.0.0.1/24             trust

## Gcloud tunneled clients
hostssl all             all             35.235.240.0/20         md5

## Gcloud VPC backend subnet
hostssl all             all             10.0.1.0/24             md5

EOF'

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 

# setup nightly pg_dump cron job
sudo -u incus incus exec db1 -- sudo -u postgres mkdir -p /var/lib/postgresql/backups

sudo -u incus incus exec db1 -- sh -c "curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg"
sudo -u incus incus exec db1 -- sh -c "echo 'deb [signed-by=/usr/share/keyrings/cloud.google.gpg] https://packages.cloud.google.com/apt cloud-sdk main' | sudo tee -a /etc/apt/sources.list.d/google-cloud-sdk.list"
sudo -u incus incus exec db1 -- sh -c "apt-get update && apt-get install -yq google-cloud-cli"


