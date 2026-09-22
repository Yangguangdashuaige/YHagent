# 【全物品】基础信息库

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/item_info_all:
    get:
      summary: 【全物品】基础信息库
      deprecated: false
      description: |-
        数据量有点大，谨慎请求。

        ----
        字段需注意：
        - 【is_get】 代表是否在交易行内，0为不可交易  1为可交易
        - 【oid】 代表的是【第三代数据ID】（一般其他接口请求这个，如果无法在交易行交易则为0）
        - 【id】 为【第二代数据ID】，目前已不适用，记录对照即可。
        - 【objectID】为【三角洲官方小程序】的数据ID，仅作为数据对应。
        ----
        资源扣除逻辑：
        - 普通请求  【20次】资源（2000+物品）
        ----
      tags:
        - 三角洲数据帝 - 基础接口/【全物品】基础数据/最新价格
      parameters:
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        desc:
                          type: string
                          description: 文本介绍
                        detail:
                          type: string
                          description: 扩展参数
                        grade:
                          type: integer
                          description: 等级
                        id:
                          type: integer
                          description: 第二代数据ID
                        is_get:
                          type: integer
                          description: 是否可交易
                        length:
                          type: integer
                          description: 长
                        objectID:
                          type: integer
                          description: 三角洲官方ID
                        objectName:
                          type: string
                          description: 名称
                        oid:
                          type: integer
                          description: 第三代数据ID
                        pic:
                          type: string
                          description: 图片
                        primaryClass:
                          type: string
                          description: 分类1
                        secondClass:
                          type: string
                          description: 分类2
                        secondClassCN:
                          type: string
                          description: 分类2中文
                        width:
                          type: integer
                          description: 宽
                      required:
                        - desc
                        - detail
                        - grade
                        - id
                        - is_get
                        - length
                        - objectID
                        - objectName
                        - oid
                        - pic
                        - primaryClass
                        - secondClass
                        - secondClassCN
                        - width
                      x-apifox-orders:
                        - desc
                        - detail
                        - grade
                        - id
                        - is_get
                        - length
                        - objectID
                        - objectName
                        - oid
                        - pic
                        - primaryClass
                        - secondClass
                        - secondClassCN
                        - width
                required:
                  - code
                  - msg
                  - count
                  - data
                x-apifox-orders:
                  - code
                  - msg
                  - count
                  - data
              example: "{\r\n    \"code\": 0,\r\n    \"msg\": \"成功\",\r\n    \"count\": 0,\r\n    \"data\": [\r\n        {\r\n            \"desc\": \"\",\r\n            \"detail\":\"{\\\"penetrationLevel\\\":6,\\\"harmRatio\\\":100,\\\"armorHarmLevel\\\":\\\"\\高\\\"}\",\r\n            \"grade\": 6,\r\n            \"id\": 1,\r\n            \"is_get\": 0,\r\n            \"length\": 1,\r\n            \"objectID\": 37150600001,\r\n            \"objectName\": \"6.8x51mm AP\",\r\n            \"oid\": 0,\r\n            \"pic\": \"https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png\",\r\n            \"primaryClass\": \"ammo\",\r\n            \"secondClass\": \"6.8x51mm\",\r\n            \"secondClassCN\": \"ammo6.8x51\",\r\n            \"width\": 1\r\n        },\r\n        {\r\n            \"desc\": \"\",\r\n            \"detail\":\"{\\\"penetrationLevel\\\":6,\\\"harmRatio\\\":100,\\\"armorHarmLevel\\\":\\\"\\高\\\"}\",\r\n            \"grade\": 6,\r\n            \"id\": 2,\r\n            \"is_get\": 0,\r\n            \"length\": 1,\r\n            \"objectID\": 37170600001,\r\n            \"objectName\": \"7.62x51mm M61\",\r\n            \"oid\": 0,\r\n            \"pic\": \"https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png\",\r\n            \"primaryClass\": \"ammo\",\r\n            \"secondClass\": \"7.62x51mm\",\r\n            \"secondClassCN\": \"ammo7.62x51\",\r\n            \"width\": 1\r\n        }\r\n    ]\r\n}"
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/【全物品】基础数据/最新价格
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-403646496-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 【全物品】最新价格数据（每周战备数据）

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/item_price_all:
    get:
      summary: 【全物品】最新价格数据（每周战备数据）
      deprecated: false
      description: |-
        数据量有点大，谨慎请求。

        注意：此接口的【id】为【第三代数据ID】

        ----
        价格更新逻辑为：
        - 高频物品 1分钟 （战备相关）
        - 中频物品  5分钟 （战备相关）
        - 低频物品  10分钟 （非战备）
        ----
        资源扣除逻辑：
        - 普通请求  【20次】资源（2000+物品）
        - 【暂不开放】战备相关  【--次】资源 （人工手动填写，每周二 10:00前更新，每周请求一次即可）
        ----

        字段需注意：
        - 【tid】 代表的是【第二代数据ID】（就是全列表基础数据的ID）
        - 【id】 代表的是【第三代数据ID】（一般其他接口请求这个）
        - 【zb_price】战备值 （每周二 10:00 前更新完毕，纯人工填写）
        - 【is_get_time】价格更新时间戳
        - 【price】价格
        - 【price_start】今日最初价格
        - 【bl】涨跌比例
        - 【day_3_price】3日价格
        - 【day_3_bl】3日涨跌比例
        - 【day_7_price】7日价格
        - 【day_7_bl】7日涨跌比例
        - 【day_30_price】30日价格
        - 【day_30_bl】30日涨跌比例
      tags:
        - 三角洲数据帝 - 基础接口/【全物品】基础数据/最新价格
      parameters:
        - name: isZb
          in: query
          description: 0为普通  1为战备数据 【暂不开放】
          required: true
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        bl:
                          type: number
                          description: 涨跌比例
                        day_30_bl:
                          type: number
                          description: 30日-涨跌比例
                        day_30_price:
                          type: integer
                          description: 30日-价格
                        day_3_bl:
                          type: number
                          description: 3日-涨跌比例
                        day_3_price:
                          type: integer
                          description: 3日-价格
                        day_7_bl:
                          type: number
                          description: 7日-涨跌比例
                        day_7_price:
                          type: integer
                          description: 7日-价格
                        id:
                          type: integer
                          description: 第三代数据ID
                        is_get_time:
                          type: integer
                          description: 更新日期
                        price:
                          type: integer
                          description: 价格
                        price_start:
                          type: integer
                          description: 今日最初价格
                        tid:
                          type: integer
                          description: 第二代数据ID
                      required:
                        - bl
                        - day_30_bl
                        - day_30_price
                        - day_3_bl
                        - day_3_price
                        - day_7_bl
                        - day_7_price
                        - id
                        - is_get_time
                        - price
                        - price_start
                        - tid
                      x-apifox-orders:
                        - bl
                        - day_30_bl
                        - day_30_price
                        - day_3_bl
                        - day_3_price
                        - day_7_bl
                        - day_7_price
                        - id
                        - is_get_time
                        - price
                        - price_start
                        - tid
                required:
                  - code
                  - msg
                  - count
                  - data
                x-apifox-orders:
                  - code
                  - msg
                  - count
                  - data
              example:
                code: 0
                msg: 成功
                count: 0
                data:
                  - bl: -4.91
                    day_30_bl: -4.91
                    day_30_price: 1324
                    day_3_bl: -4.91
                    day_3_price: 1324
                    day_7_bl: -4.91
                    day_7_price: 1324
                    id: 1
                    is_get_time: 1768208240
                    price: 1259
                    price_start: 1324
                    tid: 1170
                  - bl: 0
                    day_30_bl: 0
                    day_30_price: 51763
                    day_3_bl: 0
                    day_3_price: 51763
                    day_7_bl: 0
                    day_7_price: 51763
                    id: 2
                    is_get_time: 1768208272
                    price: 51763
                    price_start: 51763
                    tid: 712
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/【全物品】基础数据/最新价格
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-403627768-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 假账排行榜

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv3_top:
    get:
      summary: 假账排行榜
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【2次】资源

        更新频率：
        - 10分钟更新一次

        a 参数
        - acc 配件
        - helmet 头
        - armor 甲
        - chest 胸挂
        - bag 背包
        - gun 枪械
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: a
          in: query
          description: 类型
          required: false
          example: all
          schema:
            type: string
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        bl:
                          type: integer
                        grade:
                          type: integer
                        id:
                          type: integer
                        jz:
                          type: integer
                        name:
                          type: string
                        pic:
                          type: string
                        price:
                          type: integer
                        time:
                          type: integer
                      required:
                        - bl
                        - grade
                        - id
                        - jz
                        - name
                        - pic
                        - price
                        - time
                required:
                  - code
                  - message
                  - msg
                  - data
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-396183650-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 实时卡战备V4 - 多种配装逻辑！

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/jzv4_zb:
    get:
      summary: 实时卡战备V4 - 多种配装逻辑！
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        更新频率：
        - 每分钟更新一次，生成几百条数据。
        - 每小时的9 19 29 39 49 59分不执行。
        - 每天 3:00~8:00 每5分钟执行一次。

        注意：
        请不要拿V3的逻辑来看！配装名称不是固定！会根据游戏情况进行随时修改！
        【花费最少】请获取所有组的数据（不包含名字含有“兑换”的组） 然后自行排序 获取前10条即可（需要贵方自行计算）

        lv 参数
        - 0 【11W】机密配置
        - 1 【18W】机密配置
        - 2  【55W】绝密巴克什
        - 3  【60W】绝密航天
        - 5  【78W】绝密监狱

        附录：【花费最少】计算逻辑
        ```js
         // 构建"花费最少"：排除名称含"兑换"的套装，拼合所有子组，按节省(c z)排序取前10
                var allBuilds = [];
                var seenDataKeys = {};

                data.forEach(function(g) {
                  if (g.name && g.name.indexOf('兑换') === -1 && g.list && g.list.length > 0) {
                    g.list.forEach(function(b) {
                      // 生成 data 的唯一标识用于去重
                      var dataKey = JSON.stringify(b.data);
                      // 如果该 data 组合已经存在，则跳过
                      if (seenDataKeys[dataKey]) {
                        return;
                      }
                      seenDataKeys[dataKey] = 1;
                      allBuilds.push({
                        name: g.name,
                        price: b.price,
                        jz: b.jz,
                        cz: b.cz,
                        data: b.data,
                      });
                    });
                  }
                });
                allBuilds.sort(function(a, b) { return a.cz - b.cz; });
                var top10 = allBuilds.slice(0, 10);
                var hzfsGroup = {
                  name: '花费最少',
                  dateT: data[0].dateT || '',
                  list: top10.map(function(b) {
                    return {
                      price: b.price,
                      jz: b.jz,
                      cz: b.cz,
                      data: b.data,
                    };
                  }),
                };
                // 插入到列表首位（不修改原 data 避免缓存重复）
                data = [hzfsGroup].concat(data);
        ```
      tags:
        - 三角洲数据帝 - 基础接口/鼠鼠卡战备
      parameters:
        - name: lv
          in: query
          description: '0 :11w | 1: 18W | 2: 55W| 3: 60W | 5: 78W'
          required: false
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        count:
                          type: integer
                          description: 战备套装组的数量
                        date:
                          type: integer
                          description: 配置更新时间
                        dateT:
                          type: string
                          description: 配置更新时间
                        list:
                          type: array
                          items:
                            type: object
                            properties:
                              cz:
                                type: integer
                                description: 差值（假账）
                              data:
                                type: array
                                items:
                                  type: object
                                  properties:
                                    bl:
                                      type: integer
                                      description: 假账百分比，小于0是便宜
                                    exchange:
                                      type: string
                                      description: 是否可兑换，空白是市场
                                    exchange_plus:
                                      type: 'null'
                                      description: 兑换参数
                                    grade:
                                      type: integer
                                      description: 等级
                                    id:
                                      type: integer
                                      description: 唯一ID
                                    jiazhang:
                                      type: integer
                                      description: 是否是假账
                                    jz:
                                      type: integer
                                      description: 假账数值，小于0是便宜，反之为贵
                                    name:
                                      type: string
                                      description: 物品名称
                                    pic:
                                      type: string
                                      description: 图片
                                    price:
                                      type: integer
                                      description: 购买价格
                                    type:
                                      type: string
                                      description: 类型
                                  required:
                                    - bl
                                    - exchange
                                    - exchange_plus
                                    - grade
                                    - id
                                    - jiazhang
                                    - jz
                                    - name
                                    - pic
                                    - price
                                    - type
                                  x-apifox-orders:
                                    - bl
                                    - exchange
                                    - exchange_plus
                                    - grade
                                    - id
                                    - jiazhang
                                    - jz
                                    - name
                                    - pic
                                    - price
                                    - type
                              index:
                                type: integer
                                description: 排名
                              jz:
                                type: integer
                                description: 总战备价值
                              name:
                                type: string
                                description: 配装名称
                              price:
                                type: integer
                                description: 花费价格
                            required:
                              - cz
                              - data
                              - index
                              - jz
                              - name
                              - price
                            x-apifox-orders:
                              - cz
                              - data
                              - index
                              - jz
                              - name
                              - price
                          description: 套装组
                        name:
                          type: string
                          description: 套装名称（全局唯一，但是非固定，不定期改名）
                        zb:
                          type: integer
                          description: 战备值门槛
                      required:
                        - count
                        - date
                        - dateT
                        - list
                        - name
                        - zb
                      x-apifox-orders:
                        - count
                        - date
                        - dateT
                        - list
                        - name
                        - zb
                required:
                  - code
                  - message
                  - msg
                  - data
                x-apifox-orders:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - count: 10
                    date: 1778664129
                    dateT: '2026-05-13 17:22:09'
                    list:
                      - cz: -9520
                        data:
                          - bl: 2
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 509
                            jiazhang: 0
                            jz: 347
                            name: 野牛冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000005.png
                            price: 12101
                            type: 枪1
                          - bl: -13
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 257
                            jiazhang: 1
                            jz: -4692
                            name: 俄系冲锋枪精密消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000177.png
                            price: 30781
                            type: 枪1-枪口
                          - bl: 2
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 445
                            jiazhang: 0
                            jz: 247
                            name: AKS-74U突击步枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18010000010.png
                            price: 12058
                            type: 枪2
                          - bl: -15
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 128
                            jiazhang: 1
                            jz: -4090
                            name: DTK制退器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000202.png
                            price: 21757
                            type: 枪2-枪口
                          - bl: -5
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 498
                            jiazhang: 1
                            jz: -1501
                            name: X25U斜侧战斗握把
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13140000035.png
                            price: 24119
                            type: 枪2-前握把
                          - bl: 8
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 535
                            jiazhang: 0
                            jz: 161
                            name: 尼龙防弹衣 (破损)
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11050001003.png
                            price: 2164
                            type: 护甲
                        index: 1
                        jz: 112508
                        name: 花费最少
                        price: 102980
                      - cz: -9517
                        data:
                          - bl: 2
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 509
                            jiazhang: 0
                            jz: 347
                            name: 野牛冲锋枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18020000005.png
                            price: 12101
                            type: 枪1
                          - bl: -13
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 257
                            jiazhang: 1
                            jz: -4692
                            name: 俄系冲锋枪精密消音器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000177.png
                            price: 30781
                            type: 枪1-枪口
                          - bl: 2
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 445
                            jiazhang: 0
                            jz: 247
                            name: AKS-74U突击步枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18010000010.png
                            price: 12058
                            type: 枪2
                          - bl: -15
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 128
                            jiazhang: 1
                            jz: -4090
                            name: DTK制退器
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13130000202.png
                            price: 21757
                            type: 枪2-枪口
                          - bl: -5
                            exchange: ''
                            exchange_plus: null
                            grade: 4
                            id: 498
                            jiazhang: 1
                            jz: -1501
                            name: X25U斜侧战斗握把
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13140000035.png
                            price: 24119
                            type: 枪2-前握把
                          - bl: 0
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 167
                            jiazhang: 0
                            jz: 4
                            name: 奔尼帽
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/11010001003.png
                            price: 2167
                            type: 头盔
                        index: 2
                        jz: 112668
                        name: 花费最少
                        price: 102983
                    name: 花费最少
                    zb: 112500
                  - count: 10
                    date: 1778664129
                    dateT: '2026-05-13 17:22:09'
                    list:
                      - cz: 2905
                        data:
                          - bl: 2
                            exchange: ''
                            exchange_plus: null
                            grade: 1
                            id: 445
                            jiazhang: 0
                            jz: 247
                            name: AKS-74U突击步枪
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/18010000010.png
                            price: 12058
                            type: 枪1
                          - bl: -5
                            exchange: ''
                            exchange_plus: null
                            grade: 3
                            id: 517
                            jiazhang: 1
                            jz: -1099
                            name: Osight微型瞄准镜
                            pic: >-
                              https://playerhub.df.qq.com/playerhub/60004/object/13110000087.png
                            price: 18477
                            type: 枪1-瞄具
                        name: 实用优先
                        zb: 112500
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/鼠鼠卡战备
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-457269341-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 全物品列表 Pro

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/item_list_pro:
    get:
      summary: 全物品列表 Pro
      deprecated: false
      description: |-
        价格更新逻辑为：
        - 高频物品 1分钟 （战备相关）
        - 中频物品  5分钟 （战备相关）
        - 低频物品  10分钟 （非战备）
        ----
        资源扣除逻辑：
        - 请求扣除【1次】资源

        字段注意
        【oid】 代表的是【第三代数据ID】（一般其他接口请求这个，如果无法在交易行交易则为0）
        【id】 为【第二代数据ID】，目前已不适用，记录对照即可。

        新增字段 【ShopSellType】
        为推荐出售方式【已扣除 手续费、上架费、点击往前一格快速售卖】。
        第二个参数为 等级，越高越推荐交易行
        第三个字段为当前交易行 往前一个柱子的【秒出价格】

        参数说明

        key1  key2【分类索引】
        - 请通过 【全物品列表 Pro - 筛选项】进行筛选
        - 如 key1 = 钥匙   key2 = 零号大坝

        sort【排序】
        - 1    今日涨跌 从小到大
        - 1-1  今日涨跌 从小到大
        - 1-2  今日涨跌 从大到小
        - 2-1  金额 从小到大
        - 2-2   金额 从大到小
        - 3-1  价值 从小到大
        - 3-2  价值 从大到小
        - 5-1 3天涨跌  从小到大
        - 5-2 3天涨跌 从大到小
        - 7-1 7天涨跌  从小到大
        - 7-2 7天涨跌 从大到小
        - 30-1 30天涨跌  从小到大
        - 30-2 30天涨跌 从大到小


        grade【等级筛选】
        - 0~6
      tags:
        - 三角洲数据帝 - 基础接口/实时交易行 + 历史价格/全物品列表Pro
      parameters:
        - name: p
          in: query
          description: 页码
          required: true
          example: 1
          schema:
            type: integer
        - name: limit
          in: query
          description: 每页数量 10~20
          required: true
          example: 10
          schema:
            type: integer
        - name: 'n'
          in: query
          description: 名字模糊搜索 需要40字符以内，区分大小写
          required: false
          schema:
            type: string
        - name: key1
          in: query
          description: 分类索引
          required: true
          schema:
            type: string
        - name: key2
          in: query
          description: 分类索引2
          required: true
          schema:
            type: string
        - name: sort
          in: query
          description: 排序
          required: true
          example: 1-1
          schema:
            type: string
        - name: grade
          in: query
          description: 等级筛选
          required: false
          schema:
            type: integer
        - name: is_pay
          in: query
          description: 必须可在交易行上架
          required: false
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        ShopSellType:
                          type: array
                          items:
                            type: string
                        bl:
                          type: integer
                        day_30_bl:
                          type: integer
                        day_30_price:
                          type: integer
                        day_7_bl:
                          type: integer
                        day_7_price:
                          type: integer
                        grade:
                          type: integer
                        id:
                          type: integer
                        isGet:
                          type: string
                        length:
                          type: integer
                        name:
                          type: string
                        oid:
                          type: integer
                        pic:
                          type: string
                        price:
                          type: integer
                        width:
                          type: integer
                      required:
                        - ShopSellType
                        - bl
                        - day_30_bl
                        - day_30_price
                        - day_7_bl
                        - day_7_price
                        - grade
                        - id
                        - isGet
                        - length
                        - name
                        - oid
                        - pic
                        - price
                        - width
                required:
                  - code
                  - msg
                  - count
                  - data
              example:
                code: 0
                msg: 成功
                count: 59
                data:
                  - ShopSellType:
                      - 交易行上架
                      - '3'
                      - '493794'
                    bl: 0
                    day_30_bl: 0
                    day_30_price: 521227
                    day_7_bl: 0
                    day_7_price: 521227
                    grade: 6
                    id: 1293
                    isGet: '1'
                    length: 2
                    name: 黄金鳄鱼头雕像
                    oid: 1227
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/15080050185.png
                    price: 521227
                    width: 2
                  - ShopSellType:
                      - 交易行上架
                      - '3'
                      - '1416521'
                    bl: 0
                    day_30_bl: 0
                    day_30_price: 1495216
                    day_7_bl: 0
                    day_7_price: 1495216
                    grade: 6
                    id: 1292
                    isGet: '1'
                    length: 2
                    name: 名窑瓷器
                    oid: 1226
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/15080050184.png
                    price: 1495216
                    width: 4
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/实时交易行 + 历史价格/全物品列表Pro
      x-apifox-status: developing
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-430950905-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 全物品列表 Pro - 筛选项

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/item_list_pro_key:
    get:
      summary: 全物品列表 Pro - 筛选项
      deprecated: false
      description: 【不消耗次数】用于获取筛选项的关联属性，建议请求一次即可。
      tags:
        - 三角洲数据帝 - 基础接口/实时交易行 + 历史价格/全物品列表Pro
      parameters:
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        children:
                          type: array
                          items:
                            type: object
                            properties:
                              extra:
                                type: integer
                              label:
                                type: string
                              value:
                                type: integer
                            required:
                              - extra
                              - label
                              - value
                        extra:
                          type: integer
                        label:
                          type: string
                        value:
                          type: integer
                      required:
                        - children
                        - extra
                        - label
                        - value
                required:
                  - code
                  - msg
                  - count
                  - data
              example:
                code: 0
                msg: 成功
                count: 0
                data:
                  - children:
                      - extra: 0
                        label: 全部
                        value: 101
                    extra: 0
                    label: 全部
                    value: 100
                  - children:
                      - extra: 0
                        label: 全部
                        value: 2
                      - extra: 1
                        label: 步枪
                        value: 3
                      - extra: 2
                        label: 冲锋枪
                        value: 4
                      - extra: 3
                        label: 轻机枪
                        value: 5
                      - extra: 4
                        label: 霰弹枪
                        value: 6
                      - extra: 5
                        label: 精确射手步枪
                        value: 7
                      - extra: 6
                        label: 狙击步枪
                        value: 8
                      - extra: 7
                        label: 手枪
                        value: 9
                      - extra: 8
                        label: 特殊武器
                        value: 10
                    extra: 1
                    label: 枪械
                    value: 1
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/实时交易行 + 历史价格/全物品列表Pro
      x-apifox-status: developing
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-430997817-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 全物品列表/Top排行（分页）

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/item_list:
    get:
      summary: 全物品列表/Top排行（分页）
      deprecated: false
      description: |-
        价格更新逻辑为：
        - 高频物品 1分钟 （战备相关）
        - 中频物品  5分钟 （战备相关）
        - 低频物品  10分钟 （非战备）
        ----
        资源扣除逻辑：
        - 战备相关  【2次】资源 （成本巨大）
        - 非战备相关  【1次】资源
        ----
        【 zhanbei 战备分类】
        将会增加3个字段
        - zb_price    当前战备价值（每周二8~9点更新）
        - zb_jz   当前假账
        - zb_bl   当前假账比例


        【 lx 类型为 top时（Top排行）】
        - 不支持 n 模糊搜索
        - 不支持分页 p
        - limit 只支持 3或者5

        ----

        参数说明

        types 【空白为钥匙卡】
        - zhanbei 战备分类【有扩展字段】【特殊】
        - acc 配件
        - ammo 子弹
        - gun 枪械
        - collection 收集品
        - consume 消耗品
        - helmet 头盔
        - armor 护甲
        - bag 背包
        - chest 胸挂
        - key  钥匙卡


        top 【排序】
        - 1    今日涨跌 从小到大
        - 1-1  今日涨跌 从小到大
        - 1-2  今日涨跌 从大到小
        - 2-1  金额 从小到大
        - 2-2   金额 从大到小
        - 3-1  价值 从小到大
        - 3-2  价值 从大到小
        - 5-1 3天涨跌 从大到小
        - 5-2 3天涨跌 从大到小
        - 7-1 7天涨跌 从大到小
        - 7-2 7天涨跌 从大到小
        - 30-1 30天涨跌 从大到小
        - 30-2 30天涨跌 从大到小
        - 80-1 假账 从大到小
        - 80-2 假账 从大到小
        - 81-1 假账比例 从大到小
        - 82-2 假账比例 从大到小

        mid【地图筛选】一般与钥匙卡联动
        - 1 大坝
        - 2 航天 
        - 3 长弓 
        - 4 巴克什 
        - 5 潮汐监狱

        grade【等级筛选】
        - 0~6

        mtype【收集品物品类型筛选】
        - 1: "工艺藏品"
        - 2: "工具材料"
        - 3: "电子物品"
        - 4: "家居物品"
        - 5: "能源燃料"
        - 6: "医疗道具"
        - 7: "资料情报"

        新增字段 【ShopSellType】
        为推荐出售方式【已扣除 手续费、上架费、点击往前一格快速售卖】。
        第二个参数为 等级，越高越推荐交易行
        第三个字段为当前交易行 往前一个柱子的【秒出价格】
      tags:
        - 三角洲数据帝 - 基础接口/实时交易行 + 历史价格
      parameters:
        - name: lx
          in: query
          description: 数据类型【list列表  top排名】为空则默认list
          required: true
          example: list
          schema:
            type: string
        - name: p
          in: query
          description: 页码
          required: true
          example: 1
          schema:
            type: integer
        - name: limit
          in: query
          description: 每页数量 10~20
          required: true
          example: 10
          schema:
            type: integer
        - name: types
          in: query
          description: 数据类型
          required: true
          example: acc
          schema:
            type: string
        - name: top
          in: query
          description: 排序
          required: true
          example: 1-1
          schema:
            type: string
        - name: 'n'
          in: query
          description: 模糊搜索 需要40字符以内
          required: false
          schema:
            type: string
        - name: mid
          in: query
          description: 地图筛选
          required: false
          schema:
            type: integer
        - name: grade
          in: query
          description: 等级筛选
          required: false
          schema:
            type: integer
        - name: mtype
          in: query
          description: 收集品物品类型筛选
          required: false
          example: 0
          schema:
            type: integer
        - name: token
          in: query
          description: 密钥
          required: true
          example: ''
          schema:
            type: string
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                type: object
                properties:
                  code:
                    type: integer
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        bl:
                          type: number
                        day_30_bl:
                          type: number
                        day_30_price:
                          type: integer
                        day_3_bl:
                          type: number
                        day_3_price:
                          type: integer
                        day_7_bl:
                          type: number
                        day_7_price:
                          type: integer
                        desc:
                          type: string
                        grade:
                          type: integer
                        id:
                          type: integer
                        length:
                          type: integer
                        name:
                          type: string
                        objectID:
                          type: integer
                        pic:
                          type: string
                        price:
                          type: integer
                        primaryClass:
                          type: string
                        secondClass:
                          type: string
                        secondClassCN:
                          type: string
                        tid:
                          type: integer
                        width:
                          type: integer
                        zb_bl:
                          type: integer
                        zb_jz:
                          type: integer
                        zb_price:
                          type: integer
                      required:
                        - bl
                        - day_30_bl
                        - day_30_price
                        - day_3_bl
                        - day_3_price
                        - day_7_bl
                        - day_7_price
                        - desc
                        - grade
                        - id
                        - length
                        - name
                        - objectID
                        - pic
                        - price
                        - primaryClass
                        - secondClass
                        - secondClassCN
                        - tid
                        - width
                        - zb_bl
                        - zb_jz
                        - zb_price
                required:
                  - code
                  - msg
                  - count
                  - data
              examples:
                '1':
                  summary: 成功示例
                  value:
                    code: 0
                    msg: 成功
                    count: 731
                    data:
                      - ShopSellType:
                          - 交易行上架
                          - '2'
                          - '13961'
                        bl: -31.57
                        day_30_bl: 16.34
                        day_30_price: 22068
                        day_3_bl: -29.64
                        day_3_price: 36488
                        day_7_bl: -29.64
                        day_7_price: 36488
                        desc: ''
                        grade: 3
                        id: 322
                        length: 1
                        name: Cobra准直式瞄准镜
                        objectID: 13110000064
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                        price: 25673
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                        tid: 898
                        width: 1
                        zb_bl: 1
                        zb_jz: 358
                        zb_price: 25315
                      - bl: -31.1
                        day_30_bl: -31.1
                        day_30_price: 61010
                        day_3_bl: -29.19
                        day_3_price: 59360
                        day_7_bl: -31.1
                        day_7_price: 61010
                        desc: ''
                        grade: 3
                        id: 317
                        length: 2
                        name: 勇士战术枪管
                        objectID: 13020000506
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13020000506.png
                        price: 42035
                        primaryClass: acc
                        secondClass: accBarrel
                        secondClassCN: 枪管
                        tid: 841
                        width: 1
                        zb_bl: -8
                        zb_jz: -4102
                        zb_price: 46137
                      - bl: -30.43
                        day_30_bl: -30.43
                        day_30_price: 29670
                        day_3_bl: -29.41
                        day_3_price: 29240
                        day_7_bl: -30.43
                        day_7_price: 29670
                        desc: >-
                          具备全覆盖的防护性和多点挂载系统，提供强大的弹药携带能力，是突击和战斗任务中的必备装备。提供14格快速使用空间。
                        grade: 4
                        id: 398
                        length: 2
                        name: 强袭战术背心
                        objectID: 11070004001
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/11070004001.png
                        price: 20640
                        primaryClass: protect
                        secondClass: chest
                        secondClassCN: 胸挂
                        tid: 562
                        width: 2
                        zb_bl: -7
                        zb_jz: -1738
                        zb_price: 22378
                      - bl: -29.85
                        day_30_bl: -29.85
                        day_30_price: 22280
                        day_3_bl: -28.32
                        day_3_price: 21805
                        day_7_bl: -29.85
                        day_7_price: 22280
                        desc: ''
                        grade: 3
                        id: 433
                        length: 1
                        name: 通用托腮板
                        objectID: 13290000001
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13290000001.png
                        price: 15630
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                        tid: 944
                        width: 1
                        zb_bl: -8
                        zb_jz: -1500
                        zb_price: 17130
                      - bl: -29.52
                        day_30_bl: -18.88
                        day_30_price: 67542
                        day_3_bl: -29.52
                        day_3_price: 77742
                        day_7_bl: -42.68
                        day_7_price: 95592
                        desc: ''
                        grade: 3
                        id: 39
                        length: 2
                        name: 1014伸缩托（折叠）
                        objectID: 13040000133
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000133.png
                        price: 54792
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                        tid: 865
                        width: 1
                        zb_bl: -20
                        zb_jz: -13922
                        zb_price: 68714
                      - bl: -29.25
                        day_30_bl: -27.74
                        day_30_price: 22258
                        day_3_bl: -29.25
                        day_3_price: 22733
                        day_7_bl: -29.25
                        day_7_price: 22733
                        desc: ''
                        grade: 2
                        id: 143
                        length: 2
                        name: MP5伸缩枪托
                        objectID: 13040000190
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000190.png
                        price: 16083
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                        tid: 1008
                        width: 1
                        zb_bl: -17
                        zb_jz: -3507
                        zb_price: 19590
                      - bl: -29.2
                        day_30_bl: -10.54
                        day_30_price: 22383
                        day_3_bl: -29.2
                        day_3_price: 28283
                        day_7_bl: -27.7
                        day_7_price: 27693
                        desc: ''
                        grade: 2
                        id: 494
                        length: 2
                        name: G3稳定枪托
                        objectID: 13040000189
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13040000189.png
                        price: 20023
                        primaryClass: acc
                        secondClass: accStock
                        secondClassCN: 枪托
                        tid: 1007
                        width: 1
                        zb_bl: -3
                        zb_jz: -662
                        zb_price: 20685
                      - bl: -28.63
                        day_30_bl: -25.58
                        day_30_price: 22279
                        day_3_bl: -28.63
                        day_3_price: 23229
                        day_7_bl: -28.63
                        day_7_price: 23229
                        desc: ''
                        grade: 3
                        id: 153
                        length: 1
                        name: 实用消音器
                        objectID: 13130000157
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13130000157.png
                        price: 16579
                        primaryClass: acc
                        secondClass: accMuzzle
                        secondClassCN: 枪口
                        tid: 927
                        width: 1
                        zb_bl: 380
                        zb_jz: 13132
                        zb_price: 3447
                      - bl: -28.45
                        day_30_bl: -7.72
                        day_30_price: 13465
                        day_3_bl: -27.36
                        day_3_price: 17105
                        day_7_bl: -14.34
                        day_7_price: 14505
                        desc: ''
                        grade: 2
                        id: 270
                        length: 1
                        name: 俄式准直二倍瞄准镜
                        objectID: 13110000042
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000042.png
                        price: 12425
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                        tid: 1019
                        width: 1
                        zb_bl: -1
                        zb_jz: -183
                        zb_price: 12608
                      - bl: -28.19
                        day_30_bl: -26.11
                        day_30_price: 71020
                        day_3_bl: -30.16
                        day_3_price: 75140
                        day_7_bl: -30.16
                        day_7_price: 75140
                        desc: ''
                        grade: 3
                        id: 31
                        length: 2
                        name: SMG45裂变长枪管
                        objectID: 13020000523
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13020000523.png
                        price: 52480
                        primaryClass: acc
                        secondClass: accBarrel
                        secondClassCN: 枪管
                        tid: 847
                        width: 1
                        zb_bl: -7
                        zb_jz: -4172
                        zb_price: 56652
                '2':
                  summary: 成功示例
                  value:
                    code: 0
                    msg: 成功
                    count: 731
                    data:
                      - bl: -57.57
                        day_30_bl: -32.06
                        day_30_price: 24078
                        day_3_bl: -46.95
                        day_3_price: 30833
                        day_7_bl: -10.55
                        day_7_price: 18288
                        desc: ''
                        grade: 2
                        id: 1130
                        length: 1
                        name: QCQ171新式高射速化枪机
                        objectID: 13420000002
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13420000002.png
                        price: 16358
                        primaryClass: acc
                        secondClass: accFunctional
                        secondClassCN: 功能性配件
                        tid: 1080
                        width: 1
                        zb_bl: -39
                        zb_jz: -10821
                        zb_price: 27179
                      - bl: -51.64
                        day_30_bl: -27.38
                        day_30_price: 18082
                        day_3_bl: -20.08
                        day_3_price: 16432
                        day_7_bl: -15.86
                        day_7_price: 15607
                        desc: ''
                        grade: 4
                        id: 188
                        length: 1
                        name: 95长弓平台套件
                        objectID: 13220000001
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13220000001.png
                        price: 13132
                        primaryClass: acc
                        secondClass: accHandGuard
                        secondClassCN: 护木
                        tid: 779
                        width: 1
                        zb_bl: -22
                        zb_jz: -3736
                        zb_price: 16868
                      - bl: -39.34
                        day_30_bl: 112.64
                        day_30_price: 28415
                        day_3_bl: -38.24
                        day_3_price: 97835
                        day_7_bl: -39.34
                        day_7_price: 99615
                        desc: ''
                        grade: 4
                        id: 298
                        length: 1
                        name: 幻影后握把
                        objectID: 13030000111
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000111.png
                        price: 60423
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                        tid: 662
                        width: 1
                        zb_bl: -41
                        zb_jz: -42122
                        zb_price: 102545
                      - bl: -34.32
                        day_30_bl: 11.67
                        day_30_price: 22068
                        day_3_bl: -32.46
                        day_3_price: 36488
                        day_7_bl: -32.46
                        day_7_price: 36488
                        desc: ''
                        grade: 3
                        id: 322
                        length: 1
                        name: Cobra准直式瞄准镜
                        objectID: 13110000064
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000064.png
                        price: 24643
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                        tid: 898
                        width: 1
                        zb_bl: -2
                        zb_jz: -672
                        zb_price: 25315
                      - bl: -33.68
                        day_30_bl: 2.99
                        day_30_price: 65582
                        day_3_bl: -24.2
                        day_3_price: 89102
                        day_7_bl: -37.86
                        day_7_price: 108702
                        desc: ''
                        grade: 3
                        id: 184
                        length: 3
                        name: SV98升华超长枪管
                        objectID: 13020000230
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13020000230.png
                        price: 67542
                        primaryClass: acc
                        secondClass: accBarrel
                        secondClassCN: 枪管
                        tid: 788
                        width: 1
                        zb_bl: -12
                        zb_jz: -9784
                        zb_price: 77326
                      - bl: -33.36
                        day_30_bl: 0
                        day_30_price: 9257
                        day_3_bl: -5.27
                        day_3_price: 9772
                        day_7_bl: -14.3
                        day_7_price: 10802
                        desc: ''
                        grade: 3
                        id: 583
                        length: 1
                        name: okp-7反射瞄准镜
                        objectID: 13110000061
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13110000061.png
                        price: 9257
                        primaryClass: acc
                        secondClass: accScope
                        secondClassCN: 瞄具
                        tid: 895
                        width: 1
                        zb_bl: -6
                        zb_jz: -662
                        zb_price: 9919
                      - bl: -32.9
                        day_30_bl: -29.01
                        day_30_price: 71020
                        day_3_bl: -32.9
                        day_3_price: 75140
                        day_7_bl: -32.9
                        day_7_price: 75140
                        desc: ''
                        grade: 3
                        id: 31
                        length: 2
                        name: SMG45裂变长枪管
                        objectID: 13020000523
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13020000523.png
                        price: 50420
                        primaryClass: acc
                        secondClass: accBarrel
                        secondClassCN: 枪管
                        tid: 847
                        width: 1
                        zb_bl: -11
                        zb_jz: -6232
                        zb_price: 56652
                      - bl: -32.69
                        day_30_bl: -27.62
                        day_30_price: 18917
                        day_3_bl: -31.08
                        day_3_price: 19867
                        day_7_bl: -29.39
                        day_7_price: 19392
                        desc: ''
                        grade: 3
                        id: 441
                        length: 1
                        name: 实用AK后握把
                        objectID: 13030000117
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13030000117.png
                        price: 13692
                        primaryClass: acc
                        secondClass: accBackGrip
                        secondClassCN: 后握把
                        tid: 851
                        width: 1
                        zb_bl: -11
                        zb_jz: -1715
                        zb_price: 15407
                      - bl: -32.61
                        day_30_bl: -45.34
                        day_30_price: 43934
                        day_3_bl: -21.66
                        day_3_price: 30654
                        day_7_bl: -31
                        day_7_price: 34804
                        desc: ''
                        grade: 3
                        id: 32
                        length: 2
                        name: SG552骑士重枪管
                        objectID: 13020000320
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13020000320.png
                        price: 24014
                        primaryClass: acc
                        secondClass: accBarrel
                        secondClassCN: 枪管
                        tid: 793
                        width: 1
                        zb_bl: -30
                        zb_jz: -10752
                        zb_price: 34766
                      - bl: -32.57
                        day_30_bl: -38.31
                        day_30_price: 83399
                        day_3_bl: -29.28
                        day_3_price: 72749
                        day_7_bl: -25.65
                        day_7_price: 69199
                        desc: ''
                        grade: 3
                        id: 1213
                        length: 1
                        name: MK4730发弹匣
                        objectID: 13120000359
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/13120000359.png
                        price: 51449
                        primaryClass: acc
                        secondClass: accMagazine
                        secondClassCN: 弹匣
                        tid: 1276
                        width: 2
                        zb_bl: -22
                        zb_jz: -14777
                        zb_price: 66226
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/实时交易行 + 历史价格
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-397278948-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```