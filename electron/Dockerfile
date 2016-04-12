FROM debian
MAINTAINER Fan Yang <yangfan879@gmail.com>

RUN apt-get -yq update && \
    apt-get -yq install \
		ca-certificates \
        xvfb \
        libgtk2.0-0 \
        libgconf-2-4 \
        libXtst6 \
        libnss3 \
        libnotify4 \
    && apt-get -yq autoremove \
    && apt-get -yq clean \
    && rm -rf /var/lib/apt/lists/* \
    && truncate -s 0 /var/log/*log

ADD ./build/ /crawler/
ADD entrypoint.sh /crawler/
WORKDIR /crawler

ENV PATH /crawler:$PATH
CMD ["crawler-linux-amd64"]
ENTRYPOINT ["/crawler/entrypoint.sh"]
