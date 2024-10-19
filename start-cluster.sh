#!/bin/bash

# Додавання запису до файлу hosts
echo "Додавання запису до /etc/hosts..."
if ! grep -q "k3d-registry" /etc/hosts; then
    echo "127.0.0.1 k3d-registry" | sudo tee -a /etc/hosts
fi

# Створення реєстру та кластера
echo "Створення реєстру та кластера..."
k3d registry create registry --port 5005
k3d cluster create video-calling-cluster --registry-use k3d-registry:5005

# Збірка та відправка образів у реєстр
echo "Збірка та відправка образів у реєстр..."
docker build -t client:latest ./client
docker build -t server:latest ./server
docker tag client:latest k3d-registry:5005/client:latest
docker tag server:latest k3d-registry:5005/server:latest
docker push k3d-registry:5005/client:latest
docker push k3d-registry:5005/server:latest

# Застосування конфігурацій Kubernetes
echo "Застосування конфігурацій Kubernetes..."
kubectl apply -f k3d/mongodb-configmap.yaml
kubectl apply -f k3d/mongodb-secret.yaml
kubectl apply -f k3d/mongodb-deployment.yaml
kubectl apply -f k3d/mongodb-service.yaml

# Створення секрету для сертифікатів сервера
echo "Створення секрету для сертифікатів сервера..."
kubectl create secret generic server-certs \
  --from-file=certificate.crt=certs/certificate.crt \
  --from-file=private.key=certs/private.key

# Застосування конфігурацій для сервера та клієнта
kubectl apply -f k3d/server-deployment.yaml
kubectl apply -f k3d/server-service.yaml
kubectl apply -f k3d/client-deployment.yaml
kubectl apply -f k3d/client-service.yaml

# Функція для перевірки готовності поду
wait_for_pod() {
    echo "Очікування готовності $1..."
    while true; do
        POD_STATUS=$(kubectl get pods -l app=$1 -o 'jsonpath={..status.conditions[?(@.type=="Ready")].status}')
        if [ "$POD_STATUS" == "True" ]; then
            echo "Под $1 готовий!"
            break
        elif [ "$POD_STATUS" == "False" ]; then
            echo "Под $1 ще не готовий. Очікування..."
        else
            echo "Под $1 ще не створено або не має мітки 'app=$1'. Очікування..."
        fi
        sleep 5
    done
}

# Очікування готовності подів
wait_for_pod "server"
wait_for_pod "client"

# Налаштування перенаправлення портів
echo "Налаштування перенаправлення портів..."
kubectl port-forward --address 0.0.0.0 service/video-calling-client-service 3000:3000 &
kubectl port-forward --address 0.0.0.0 service/video-calling-server-service 8080:8080 &

echo "Кластер запущено та налаштовано. Сервіси доступні за адресами:"
echo "Клієнт: http://localhost:3000"
echo "Сервер: http://localhost:8080"
