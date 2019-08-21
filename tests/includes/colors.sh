red() {
    if tput setaf 1 &> /dev/null; then
        tput sgr0
        echo "$(tput setaf 1)${1}$(tput sgr0)"
    else
        echo "${1}"
    fi
}

green() {
    if tput setaf 1 &> /dev/null; then
        tput sgr0
        echo "$(tput setaf 2)${1}$(tput sgr0)"
    else
        echo "${1}"
    fi
}
