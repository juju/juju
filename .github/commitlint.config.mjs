export default {
    extends: ['@commitlint/config-conventional'],
    rules: {
        /* The 1 means this is a warning, 2 would mean an error. */
        'body-max-line-length': [1, 'always', 100],
        'footer-max-line-length': [1, 'always', 100],
    },
};
