incus launch images:debian/12/cloud db1
incus exec db1 -- apt install postgresql
incus snapshot create db1 fresh-postgres15-install


# https://wiki.debian.org/PostgreSql
sudo adduser --system --shell /bin/bash --home /home/sportsoil sportsoil
sudo -u postgres createuser sportsoil
createdb -O sportsoil sportsoil

# add pg_hba entry for client and accept tcp connections


# add incus network forward to send traffic from host to instance
# incus network forward create incusbr0 <internal address> 
# incus network forward port add incusbr0 10.0.1.5 tcp 5432 10.233.59.196

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 