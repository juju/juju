red() {
    if [ -n "${TERM}" ]; then
        if which tput >/dev/null 2>&1; then
            tput sgr0
            echo "$(tput setaf 1)${1}$(tput sgr0)"
            return
        fi
    fi
    echo "${1}"
}

green() {
    if [ -n "${TERM}" ]; then
        if which tput >/dev/null 2>&1; then
            tput sgr0
            echo "$(tput setaf 2)${1}$(tput sgr0)"
            return
        fi
    fi
    echo "${1}"
}
