rnd_str() {
    # shellcheck disable=SC2018
    rnd=$(head /dev/urandom | tr -dc a-z0-9 | head -c 8; echo '')
    echo "${rnd}"
}