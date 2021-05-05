# Video Call Over RTC/WebSockets Using NATs

# setup
## install and run nats 
`sudo snap install nats-server`
`nats-server`

## requires SSL certs to operate (browsers don't like to expose your webcam over http)
### generate cert
`openssl req  -new  -newkey rsa:2048  -nodes  -keyout localhost.key  -out localhost.csr`
plug in your public IP
### sign cert
`openssl  x509  -req  -days 365  -in localhost.csr  -signkey localhost.key  -out localhost.crt`

### update nginx
	server {
	    server_name xx.xx.xx.xxx;
	    ssl_certificate /etc/ssl/certs/localhost.crt;
	    ssl_certificate_key /etc/ssl/private/localhost.key;
	    listen 80;
	    listen 443 ssl http2;
	    listen [::]:443 ssl http2;
	    ssl_protocols TLSv1.2 TLSv1.1 TLSv1;
	    location / {
	        proxy_set_header   X-Forwarded-For $remote_addr;
	        proxy_set_header   Host $http_host;
	    	proxy_set_header Upgrade $http_upgrade;
	    	proxy_http_version 1.1;
	    	proxy_set_header Connection "Upgrade";
	        proxy_pass         "http://127.0.0.1:8000";
	    } 
	}

### go chit chat
https://xx.xx.xx.xxx/?userID=peer2&peerID=peer1
https://xx.xx.xx.xxx/?userID=peer1&peerID=peer2


project was based and inspired by https://mattbutterfield.com/blog/2021-05-02-adding-video-chat