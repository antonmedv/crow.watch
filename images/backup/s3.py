#!/usr/bin/env python3
"""Simple S3 helper for Linode Object Storage."""

import os
import sys

import boto3


def client():
    return boto3.client(
        "s3",
        endpoint_url=f"https://{os.environ['LINODE_CLUSTER']}",
        aws_access_key_id=os.environ["LINODE_CLI_OBJ_ACCESS_KEY"],
        aws_secret_access_key=os.environ["LINODE_CLI_OBJ_SECRET_KEY"],
    )


def put(local_path, bucket, key):
    client().upload_file(local_path, bucket, key)


def ls(bucket, prefix):
    s3 = client()
    paginator = s3.get_paginator("list_objects_v2")
    for page in paginator.paginate(Bucket=bucket, Prefix=prefix):
        for obj in page.get("Contents", []):
            print(obj["Key"])


def rm(bucket, key):
    client().delete_object(Bucket=bucket, Key=key)


if __name__ == "__main__":
    cmd = sys.argv[1]
    if cmd == "put":
        put(sys.argv[2], sys.argv[3], sys.argv[4])
    elif cmd == "ls":
        ls(sys.argv[2], sys.argv[3])
    elif cmd == "rm":
        rm(sys.argv[2], sys.argv[3])
    else:
        print(f"Unknown command: {cmd}", file=sys.stderr)
        sys.exit(1)
