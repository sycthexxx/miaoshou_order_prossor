# Match package-lock.json's Playwright version; upgrade them together.
FROM mcr.microsoft.com/playwright:v1.63.0-noble
USER root
RUN apt-get update && apt-get install -y --no-install-recommends xvfb x11vnc novnc websockify openbox fonts-noto-cjk x11-utils && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY internal/adapters/miaoshou/web/worker ./browser
COPY deploy/browser/entrypoint.sh /usr/local/bin/order-browser
RUN chmod 755 /usr/local/bin/order-browser
ENV ORDER_DATA_DIR=/data BROWSER_PORT=18080 DISPLAY=:99 HOME=/tmp/browser-home
EXPOSE 18080 6080
ENTRYPOINT ["/usr/local/bin/order-browser"]
