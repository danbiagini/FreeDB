#! /bin/bash

# A bash function to connect to an incus instance. 

incus_connect_instance() {
    sudo -u incus incus exec "$1" -- bash
}
