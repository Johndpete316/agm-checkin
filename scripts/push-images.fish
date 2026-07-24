#!/usr/bin/env fish

# Usage: ./push-images.fish [--api | --frontend]
# With no flag, both images are built and deployed.

set BUILD_API 1
set BUILD_FE  1

for arg in $argv
    switch $arg
        case --api
            set BUILD_FE 0
        case --frontend
            set BUILD_API 0
        case '*'
            echo "Unknown flag: $arg"
            echo "Usage: ./push-images.fish [--api | --frontend]"
            exit 1
    end
end

# All nodes including the control plane — k3s schedules pods on the control plane by default
set NODES \
    "ubuntu@k8s-cp" \
    "ubuntu@k8s-worker-1" \
    "ubuntu@k8s-worker-2" \
    "ubuntu@k8s-worker-3"

# Update VITE_API_URL to wherever your API tunnel will live
set VITE_API_URL "https://apicheckin.reduxit.net"

set API_IMAGE "agm-api:latest"
set FE_IMAGE  "agm-frontend:latest"

echo "==> Building images..."

if test $BUILD_API -eq 1
    docker build -t $API_IMAGE ../agm-checkin-api
    or begin; echo "API build failed"; exit 1; end
end

if test $BUILD_FE -eq 1
    docker build \
        --build-arg VITE_API_URL=$VITE_API_URL \
        -t $FE_IMAGE \
        ../agm-checkin-frontend
    or begin; echo "Frontend build failed"; exit 1; end
end

echo "==> Importing images to k3s nodes..."
for node in $NODES
    echo "--> $node"
    if test $BUILD_API -eq 1
        docker save $API_IMAGE | gzip | ssh $node "sudo k3s ctr images import -"
    end
    if test $BUILD_FE -eq 1
        docker save $FE_IMAGE  | gzip | ssh $node "sudo k3s ctr images import -"
    end
end

echo ""
echo "==> Deploying via Helm..."
helm upgrade --install agm-checkin ../helm/agm-checkin \
    -f ../helm/agm-checkin/values.secret.yaml
or begin; echo "Helm upgrade failed"; exit 1; end

echo ""
echo "==> Restarting deployments to pick up new images..."
if test $BUILD_API -eq 1
    kubectl rollout restart deployment/agm-checkin-api
end
if test $BUILD_FE -eq 1
    kubectl rollout restart deployment/agm-checkin-frontend
end

if test $BUILD_API -eq 1
    kubectl rollout status deployment/agm-checkin-api --timeout=120s
end
if test $BUILD_FE -eq 1
    kubectl rollout status deployment/agm-checkin-frontend --timeout=120s
end

echo ""
echo "Done. All pods are running the new images."
