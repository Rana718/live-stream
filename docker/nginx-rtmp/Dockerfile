FROM tiangolo/nginx-rtmp:latest

# Create directories with proper permissions
RUN mkdir -p /tmp/hls /tmp/recordings && \
    chmod 777 /tmp/hls /tmp/recordings && \
    chown -R nobody:nogroup /tmp/hls /tmp/recordings

# Copy custom nginx config
COPY nginx.conf /etc/nginx/nginx.conf

# Expose ports
EXPOSE 1935 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s \
    CMD curl -f http://localhost/stat || exit 1
