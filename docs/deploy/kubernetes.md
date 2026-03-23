# Kubernetes Deployment With Helm

Kubernetes users should deploy `claude-oauth-proxy` with the Helm chart shipped in this repository.

Raw Kubernetes manifests are not the supported deployment surface for this product repo.

## Before You Install

The proxy stores a refreshable Claude OAuth session on disk, so the safe Kubernetes shape is:

- one replica
- a writable volume for `tokens.json`
- a one-time headless login bootstrap after the pod is running

Do not scale the chart above one replica unless the token model changes. The current OAuth refresh flow is single-writer.

## Add The Chart Repository

Once the Helm publish workflow has populated GitHub Pages, add the chart repository:

```bash
helm repo add claude-oauth-proxy https://bonztm.github.io/claude-oauth-proxy
helm repo update
```

## Create The API Key Secret

Create the Kubernetes secret that holds the local API key clients will use:

```bash
kubectl create namespace claude-oauth-proxy

kubectl create secret generic claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --from-literal=api-key=sk-proxy-local-key
```

## Install The Chart

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --create-namespace \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy
```

If you already have a claim for persistent runtime state, point the chart at it:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --create-namespace \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy \
  --set persistence.existingClaim=claude-oauth-proxy-data
```

## Bootstrap Claude Login In The Running Pod

After the release is up, start a headless login session directly inside the running container:

```bash
kubectl exec -it -n claude-oauth-proxy deployment/claude-oauth-proxy -- \
  /usr/local/bin/claude-oauth-proxy login --no-browser
```

That command prints the Claude OAuth URL, waits for the pasted `?code=...` value, and writes the resulting token file into the persistent volume.

The container image is minimal, so run the binary directly with `kubectl exec` instead of expecting `/bin/sh` to be present.

## Access The Proxy

Port-forward the service:

```bash
kubectl port-forward -n claude-oauth-proxy svc/claude-oauth-proxy 9999:9999
```

Then point your client at:

```bash
export OPENAI_BASE_URL="http://127.0.0.1:9999/v1"
export OPENAI_API_KEY="sk-proxy-local-key"
```

## Ingress Example

If you want a DNS-backed entrypoint:

```bash
helm upgrade --install claude-oauth-proxy claude-oauth-proxy/claude-oauth-proxy \
  --namespace claude-oauth-proxy \
  --create-namespace \
  --set config.apiKey.existingSecret.name=claude-oauth-proxy \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=claude-proxy.example.com
```

## Ongoing Operations

- `GET /livez` is the liveness endpoint
- `GET /readyz` is the readiness endpoint
- keep the token volume writable so refreshes can be persisted
- if you rotate the local API key, update clients to match

---

**See also:** [README](../../README.md) · [Docker Compose deployment](docker-compose.md) · [Helm chart details](../../charts/claude-oauth-proxy/README.md) · [Configuration reference](../configuration.md)
