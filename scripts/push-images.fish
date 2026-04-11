#!/usr/bin/env fish

# All nodes including the control plane — k3s schedules pods on the control plane by default
set NODES \
    "ubuntu@k8s-cp" \
    "ubuntu@k8s-worker-1" \
    "ubuntu@k8s-worker-2" \
    "ubuntu@k8s-worker-3"

# Update VITE_API_URL to wherever your API tunnel will live
set VITE_API_URL "https://apicheckin.reduxit.net"

set API_IMAGE   "agm-api:latest"
set FE_IMAGE    "agm-frontend:latest"

echo "==> Building images..."
docker build -t $API_IMAGE ../agm-checkin-api
or begin; echo "API build failed"; exit 1; end

docker build \
    --build-arg VITE_API_URL=$VITE_API_URL \
    -t $FE_IMAGE \
    ../agm-checkin-frontend
or begin; echo "Frontend build failed"; exit 1; end

echo "==> Importing images to k3s nodes..."
for node in $NODES
    echo "--> $node"
    docker save $API_IMAGE | gzip | ssh $node "sudo k3s ctr images import -"
    docker save $FE_IMAGE  | gzip | ssh $node "sudo k3s ctr images import -"
end

echo ""
echo "Done. Images are available on all nodes."
echo "Deploy with:"
echo "  helm upgrade --install agm-checkin ../helm/agm-checkin \\"
echo "    -f ../helm/agm-checkin/values.secret.yaml"
