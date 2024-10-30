incus launch images:debian/12/cloud proxy1 --profile v1
incus exec proxy1 -- apt update
incus exec proxy1 -- apt install -yq git curl

incus exec proxy1 -- sudo adduser --system --shell /bin/bash --home /home/traefik traefik
incus exec proxy1 -- sudo usermod -aG sudo traefik
# setup the postgres user with bash and PATH after the gcloud install
incus exec proxy1 -- cp .bashrc /home/traefik/
incus exec proxy1 -- cp .profile /home/traefik/
incus exec proxy1 -- chown traefik /home/traefik/.bashrc 
incus exec proxy1 -- chown traefik /home/traefik/.profile

incus exec proxy1 -- sudo -u -i git clone https://github.com/danbiagini/FreeDB.git

# check version of traefik
#incus exec proxy1 -- sudo -u traefik -i curl -L 'https://github.com/traefik/traefik/releases/download/v3.1.6/traefik_v3.1.6_linux_amd64.tar.gz' > traefik_v3.1.6.tar.gz
#incus exec proxy1 -- sudo -u traefik -i tar -xzvf traefik_v3.1.6.tar.gz 

#incus exec proxy1 -- sudo -u traefik -i traefik --configFile=FreeDB/config/traefik.toml

incus network forward port add incusbr0 10.0.1.5 tcp 80,443,8080 10.233.59.32

# https://cloud.google.com/iap/docs/using-tcp-forwarding#create-firewall-rule
# to connect from G CloudShell 

# to connect to dash
# gcloud compute start-iap-tunnel freedb 8080 --local-host-port=localhost:8080
# 'preview on port 8080' in cloud shell 