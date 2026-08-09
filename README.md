# bifrost插件仓库

## 版本信息
- **tag**: transports/v1.6.9
- **core**: v1.7.7

## 插件一览

- **pil-masking**:  数据脱敏
- **pricing-override**: 价格重写
- **vision-extension**: 视觉扩展

## 升级说明

- [ ] 确认新版本的Dockerfile流程是否与当前版本一致
- [ ] 同步`Dockerfile`的`BIFROST_TAG`为新版本
- [ ] 同步`plugins`下面的各个插件的go.mod中`github.com/maximhq/bifrost/core`版本为新版本