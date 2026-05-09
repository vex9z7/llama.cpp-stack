FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    HF_HOME=/hf-cache

RUN pip install --no-cache-dir --upgrade pip huggingface_hub

WORKDIR /work
ENTRYPOINT ["hf"]
