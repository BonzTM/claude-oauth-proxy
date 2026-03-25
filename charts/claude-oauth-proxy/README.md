# claude-oauth-proxy Helm Chart

Deploy `claude-oauth-proxy` on Kubernetes with a single replica, a persistent token volume, and an OpenAI-compatible service endpoint.

## Quick Start

Create an API key secret:

```bash
kubectl create namespace claude-oauth-proxy

kubectl create secret generic claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --from-literal=api-key=sk-proxy-local-key
```

Add the chart repository and install the chart:

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update

helm install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy
```

Bootstrap Claude login with a headless `kubectl exec` session:

```bash
kubectl exec -it -n claude-oauth-proxy deployment/claude-oauth-proxy -- \
  /usr/local/bin/claude-oauth-proxy login --no-browser
```

Forward the service locally:

```bash
kubectl port-forward -n claude-oauth-proxy svc/claude-oauth-proxy 9999:9999
```

## Persistence

The chart stores the token file on a writable volume. The default path inside the container is:

```text
/var/lib/claude-oauth-proxy/tokens.json
```

Use an existing claim if you already manage storage:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy \
  --set persistence.existingClaim=claude-oauth-proxy-data
```

## Ingress

Enable Ingress if you want a DNS-backed entrypoint:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=claude-proxy.example.com
```

## Values To Know

- `config.apiKey.value`
- `config.apiKey.existingSecret.name`
- `persistence.enabled`
- `persistence.existingClaim`
- `config.extraEnv`
- `config.extraEnvFrom`
- `ingress.enabled`
- `image.tag`

## Image Tags

The chart supports the same image channels as the published container repository:

- `latest` or a release number like `1.2.3` for published releases
- `nightly` for the moving `main` branch channel
- `develop` for the moving non-`main` branch channel

Example:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy \
  --set image.tag=nightly
```

## Environment Variable Precedence

The chart sets a few defaults for you, including:

- `CLAUDE_OAUTH_PROXY_LISTEN_ADDR`
- `CLAUDE_OAUTH_PROXY_TOKEN_FILE`
- `CLAUDE_OAUTH_PROXY_API_KEY`

If you define the same keys in `config.extraEnv`, your values take precedence and the chart does not emit the built-in versions of those variables.

`config.extraEnvFrom` remains additive. Use `config.extraEnv` when you want to override a built-in variable directly.

Example:

```yaml
config:
  extraEnv:
    CLAUDE_OAUTH_PROXY_API_KEY: my-custom-local-key
    CLAUDE_OAUTH_PROXY_TOKEN_FILE: /data/custom/tokens.json
    CLAUDE_OAUTH_PROXY_COST_TRACKING: "true"
```

Equivalent install command:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --set-string config.extraEnv.CLAUDE_OAUTH_PROXY_API_KEY=my-custom-local-key \
  --set-string config.extraEnv.CLAUDE_OAUTH_PROXY_TOKEN_FILE=/data/custom/tokens.json \
  --set-string config.extraEnv.CLAUDE_OAUTH_PROXY_COST_TRACKING=true
```

When you override `CLAUDE_OAUTH_PROXY_API_KEY` this way, the chart stops emitting the built-in API key env var and does not require the chart-managed API key Secret.

## Upgrade

```bash
helm upgrade claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy -n claude-oauth-proxy
```

## Uninstall

```bash
helm uninstall claude-oauth-proxy -n claude-oauth-proxy
```

---

**See also:** [README](../../README.md) · [Kubernetes deployment guide](../../docs/deploy/kubernetes.md) · [Configuration reference](../../docs/configuration.md)
