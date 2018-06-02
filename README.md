## Introduction
Tenant extract discussions from [Douban Group](https://www.douban.com/group/guide?guide=1) and show them based on user configurable parameters: group name, search key string, maxium search discussion number and maxium workers used to search concurrently. Initial intention is to filter rent information in [shanghaizufang](https://www.douban.com/group/shanghaizufang/), but can search in other groups too.

## Install
```bash
go get -u github.com/jaysinco/tenant
```

## Usage
```bash
tenant -h
Usage: tenant [-e][-w] [douban group id] [max page] [search regexp]
  -e bool
      email query result, default FALSE 
  -w int
      max network fetch worker, default one goroutine 
```