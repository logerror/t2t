FROM reg.deeproute.ai/deeproute-public/ubuntu:20.04-basetool-v2

WORKDIR /app

COPY . /app/
#RUN ln -sf /app/Shanghai /etc/localtime
#RUN echo 'Asia/Shanghai' > /etc/timezone
ENTRYPOINT ["/app/t2t-server"]