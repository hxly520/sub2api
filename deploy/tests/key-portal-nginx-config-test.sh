#!/bin/sh
set -eu

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
config="$repo_root/deploy/nginx/key.52token.org.conf.example"
snippet="$repo_root/deploy/nginx/sub2api-key-proxy.inc.example"

require() {
    grep -Fq "$2" "$1" || {
        printf 'missing required nginx contract: %s\n' "$2" >&2
        exit 1
    }
}

require "$config" 'server_name key.52token.org;'
require "$config" 'if ($sub2api_cloudflare_peer = 0) { return 444; }'
require "$config" 'return 302 /card;'
require "$config" 'location = /api/v1/public/link-cards/activate {'
require "$config" 'location ^~ /api/v1/public/link-cards/ {'
require "$config" 'access_log off;'
require "$config" 'limit_req zone=sub2api_key_activate'
require "$config" 'location = /health { return 404; }'
require "$config" 'location / { return 404; }'
require "$snippet" 'proxy_pass http://127.0.0.1:8080;'
require "$snippet" 'proxy_set_header X-Forwarded-For $remote_addr;'
require "$snippet" 'proxy_set_header X-Forwarded-Proto https;'

if grep -Fq 'gpt-codex.top' "$config" "$snippet"; then
    printf 'retired domain found in quota-card nginx config\n' >&2
    exit 1
fi

printf 'quota-card nginx contract ok\n'
