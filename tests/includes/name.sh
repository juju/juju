rnd_name() {
    local prefix

    prefix=${1}

    rnd=$(head /dev/urandom | tr -dc a-z0-9 | head -c 8; echo '')
    echo "${prefix}${rnd}"
}