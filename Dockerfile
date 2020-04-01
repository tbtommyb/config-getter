FROM debian
COPY ./config-getter /config-getter
ENTRYPOINT /config-getter
