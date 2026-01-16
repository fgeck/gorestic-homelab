# Kubernetes Deployment

Deploy gorestic-homelab as a CronJob in Kubernetes.

## Quick Start

1. Edit the configuration files:
   - `secret.yaml` - Set your passwords and tokens
   - `configmap.yaml` - Configure backup settings
   - `pvc.yaml` - Adjust storage settings
   - `cronjob.yaml` - Set schedule and resources

2. Deploy using kubectl:
   ```bash
   kubectl apply -k . -n your-namespace
   ```

   Or apply individually:
   ```bash
   kubectl apply -f secret.yaml -n your-namespace
   kubectl apply -f configmap.yaml -n your-namespace
   kubectl apply -f pvc.yaml -n your-namespace
   kubectl apply -f cronjob.yaml -n your-namespace
   ```

3. Trigger a manual backup:
   ```bash
   kubectl create job --from=cronjob/gorestic-backup manual-backup -n your-namespace
   ```

## Using Existing PVC

If you want to back up data from an existing PVC, update `cronjob.yaml`:

```yaml
volumes:
  - name: data
    persistentVolumeClaim:
      claimName: your-existing-pvc  # Change this
```

And skip creating `pvc.yaml`.

## SSH Key for Remote Shutdown

If using SSH shutdown feature, create a secret with your SSH key:

```bash
kubectl create secret generic ssh-key \
  --from-file=id_rsa=/path/to/your/key \
  -n your-namespace
```

Then uncomment the SSH volume sections in `cronjob.yaml`.

## Monitoring

Check CronJob status:
```bash
kubectl get cronjob gorestic-backup -n your-namespace
```

View recent jobs:
```bash
kubectl get jobs -n your-namespace -l app.kubernetes.io/name=gorestic-homelab
```

View logs from latest job:
```bash
kubectl logs -l app.kubernetes.io/component=backup -n your-namespace --tail=100
```

## Customization with Kustomize

Create an overlay for environment-specific settings:

```bash
mkdir -p overlays/production
```

Create `overlays/production/kustomization.yaml`:
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

resources:
  - ../../deploy/kubernetes

patches:
  - patch: |-
      - op: replace
        path: /spec/schedule
        value: "0 2 * * *"
    target:
      kind: CronJob
      name: gorestic-backup

images:
  - name: ghcr.io/fgeck/gorestic-homelab
    newTag: v1.0.0
```

Deploy:
```bash
kubectl apply -k overlays/production
```
