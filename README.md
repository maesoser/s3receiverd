# ssrxd

Simple Storage Receiver Daemon (ssrxd) is a tiny HTTP server that is able to receive and store logs sent by an AWS-S3 compatible client using either putObject or uploadPart.

It was designed as a simple receiver for [Cloudflare Logs](https://developers.cloudflare.com/logs/)


## How to create a logpush job in Cloudflare

The steps needed to create an s3 compatible logpush job can be found on [Cloudflare's developers docs](https://developers.cloudflare.com/logs/get-started/enable-destinations/s3-compatible-endpoints)

```bash
curl -s -X POST \
    https://api.cloudflare.com/client/v4/zones/<ZONE_ID>/logpush/jobs \
    -d '{
	"name": "<DOMAIN_NAME>",
	"destination_conf": "s3://<BUCKET-NAME>/<BUCKET-PATH>?region=<REGION>&access-key-id=<ACCESS-KEY-ID>&secret-access-key=<SECRET-ACCESS-KEY>&endpoint=<ENDPOINT-URL>",
	"logpull_options": "fields=RayID,EdgeStartTimestamp&timestamps=rfc3339",
	"dataset": "http_requests"
    }' | jq .
```

In this specific example, the API call will be the following:

```bash
curl -s -X POST \
    https://api.cloudflare.com/client/v4/zones/<ZONE_ID>/logpush/jobs \
    -d '{
	"name": "s3_example_job",
	"destination_conf": "s3://logs/?region=us-west-2&access-key-id=AKIAI44QH8DHBEXAMPLE&secret-access-key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY&endpoint=logs.domain.com",
	"logpull_options": "fields=RayID,EdgeStartTimestamp&timestamps=rfc3339",
	"dataset": "http_requests"
    }' | jq .
```

## Docker Compose file

```yaml
  ssrxd:
    container_name: logreceiver
    restart: unless-stopped
    mem_limit: 64m
    cpu_count: 1
    logging:
      options:
        max-size: "1m"
        max-file: "1"
    build:
     context: ./containers/logrecv
     dockerfile: Dockerfile
    volumes:
     - ./data/logrecv:/logs:z
     - ./data/logrecv/spectrum:/spectrum:z
    environment:
     - ROOT_FOLDER=/logs
     - SECRET=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     - ACCESS_KEY=AKIAI44QH8DHBEXAMPLE
```