FROM php:8.3-cli AS php-base

RUN  --mount=type=bind,from=mlocati/php-extension-installer:latest,source=/usr/bin/install-php-extensions,target=/usr/local/bin/install-php-extensions \
      install-php-extensions bcmath @composer intl opcache openswoole pdo_sqlite pcntl

EXPOSE 9501
