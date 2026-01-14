Build traffic images from the repo's src/traffic:
  docker build -t lab-traffic-server -f src/traffic/Dockerfile.server src/traffic
  docker build -t lab-traffic-client -f src/traffic/Dockerfile.client src/traffic
