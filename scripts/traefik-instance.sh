incus launch images:debian/12/cloud proxy1 --profile v1
incus exec proxy1 -- apt update
incus exec proxy1 -- apt install -yq git curl

incus exec proxy1 -- sudo adduser --system --shell /bin/bash --home /home/traefik traefik
# setup the postgres user with bash and PATH after the gcloud install
incus exec proxy1 -- cp .bashrc /home/traefik/
incus exec proxy1 -- cp .profile /home/traefik/

# check version of traefik
#incus exec proxy1 -- sudo -u traefik -i curl -L 'https://github.com/traefik/traefik/releases/download/v3.1.6/traefik_v3.1.6_linux_amd64.tar.gz' > traefik_v3.1.6.tar.gz
#incus exec proxy1 -- sudo -u traefik -i tar -xzvf traefik_v3.1.6.tar.gz 

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 
