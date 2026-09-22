# 特勤处制造利润Pro - 自有源计算

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/manufacturePro:
    get:
      summary: 特勤处制造利润Pro - 自有源计算
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        更新频率：
        - 每10分钟更新一次

        t 参数
        - 1 技术中心
        - 2 工作台
        - 3 制药台
        - 4 防具台

        l 参数
        - 1 等级1
        - 2 等级2
        - 3 等级3

        字段解释：
        - unlockLevel 解锁等级
        - period 制造最小时长
        - priceMax 未扣除手续费价格
        - sxf 手续费（手续费+上架费）
        - price  真实到手价格（已扣除手续费）
        - price_hour 每小时收益价格（已扣除手续费）


        **注意：不同等级的部门收益不一样！（生产时间不一样）**
      tags:
        - 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      parameters:
        - name: t
          in: query
          description: 1 技术中心 2工作台 3制药台 4防具台
          required: false
          example: 1
          schema:
            type: integer
        - name: l
          in: query
          description: 部门等级
          required: false
          example: 3
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
                        grade:
                          type: integer
                        id:
                          type: integer
                        isGet:
                          type: integer
                        is_pay:
                          type: integer
                        name:
                          type: string
                        objectID:
                          type: integer
                        oid:
                          type: integer
                        period:
                          type: number
                        pic:
                          type: string
                        price:
                          type: integer
                        priceMax:
                          type: integer
                        price_hour:
                          type: integer
                        primaryClass:
                          type: string
                        secondClass:
                          type: string
                        secondClassCN:
                          type: string
                        sxf:
                          type: integer
                        unlockLevel:
                          type: integer
                      required:
                        - grade
                        - id
                        - isGet
                        - is_pay
                        - name
                        - objectID
                        - oid
                        - period
                        - pic
                        - price
                        - priceMax
                        - price_hour
                        - primaryClass
                        - secondClass
                        - secondClassCN
                        - sxf
                        - unlockLevel
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - grade: 4
                    id: 776
                    isGet: 1
                    is_pay: 1
                    name: OLIGHT WARRIOR 3S战术手电
                    objectID: 13160000018
                    oid: 0
                    period: 4.5
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/13160000018.png
                    price: 16396
                    priceMax: 71469
                    price_hour: 3643
                    primaryClass: acc
                    secondClass: accFunctional
                    secondClassCN: 功能性配件
                    sxf: 2131
                    unlockLevel: 1
                  - grade: 4
                    id: 785
                    isGet: 1
                    is_pay: 1
                    name: 蜂网遮光罩
                    objectID: 13370000001
                    oid: 0
                    period: 4.5
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/13370000001.png
                    price: -7172
                    priceMax: 74068
                    price_hour: -1593
                    primaryClass: acc
                    secondClass: accFunctional
                    secondClassCN: 功能性配件
                    sxf: -932
                    unlockLevel: 1
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-422150879-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 军需处兑换 - 成本

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/exchange:
    get:
      summary: 军需处兑换 - 成本
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        更新频率：
        - 每10分钟更新一次

        t 参数
        - 1 战斗部门
        - 2 医疗部门
        - 3 后勤部门
        - 4 战术部门
        - 5 研发部门

        字段解释：
        - price 兑换成本
        - profit  与交易行的差值
        - purchaseCount  可兑换次数
        - purchaseDuration 兑换刷新小时
        - task  解锁兑换要求
        - isGet 是否交易行存在
        - is_pay 是否可上架交易行
        - sxf 手续费
        - priceMax 出售价格
        - priceProfit 出售利润
        - count 兑换数量
        - unlockLevel 解锁要求
      tags:
        - 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      parameters:
        - name: t
          in: query
          description: 1 战斗部门 2 医疗部门 3 后勤部门 4 战术部门 5 研发部门
          required: false
          example: 1
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
                        grade:
                          type: integer
                        id:
                          type: integer
                        isGet:
                          type: integer
                        is_pay:
                          type: integer
                        name:
                          type: string
                        objectID:
                          type: integer
                        oid:
                          type: integer
                        pic:
                          type: string
                        price:
                          type: integer
                        priceMax:
                          type: integer
                        priceProfit:
                          type: integer
                        primaryClass:
                          type: string
                        profit:
                          type: integer
                        purchaseCount:
                          type: string
                        purchaseDuration:
                          type: string
                        secondClass:
                          type: string
                        secondClassCN:
                          type: string
                        sxf:
                          type: integer
                        task:
                          type: string
                        unlockLevel:
                          type: string
                      required:
                        - count
                        - grade
                        - id
                        - isGet
                        - is_pay
                        - name
                        - objectID
                        - oid
                        - pic
                        - price
                        - priceMax
                        - priceProfit
                        - primaryClass
                        - profit
                        - purchaseCount
                        - purchaseDuration
                        - secondClass
                        - secondClassCN
                        - sxf
                        - task
                        - unlockLevel
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - count: 1
                    grade: 1
                    id: 70
                    isGet: 1
                    is_pay: 1
                    name: 5.56x45mm RRLP
                    objectID: 37100100001
                    oid: 1063
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                    price: 35
                    priceMax: 35
                    priceProfit: 31
                    primaryClass: ammo
                    profit: 0
                    purchaseCount: 无限制
                    purchaseDuration: 无限制
                    secondClass: ammo5.56x45
                    secondClassCN: 5.56x45mm
                    sxf: 4
                    task: ''
                    unlockLevel: 0级解锁
                  - count: 1
                    grade: 2
                    id: 58
                    isGet: 1
                    is_pay: 1
                    name: 5.56x45mm FMJ
                    objectID: 37100200001
                    oid: 1018
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                    price: 79
                    priceMax: 79
                    priceProfit: 69
                    primaryClass: ammo
                    profit: 0
                    purchaseCount: 无限制
                    purchaseDuration: 无限制
                    secondClass: ammo5.56x45
                    secondClassCN: 5.56x45mm
                    sxf: 10
                    task: ''
                    unlockLevel: 0级解锁
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-421909457-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 军需处兑换 - 子弹收益

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/exchange_pay:
    get:
      summary: 军需处兑换 - 子弹收益
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        更新频率：
        - 每10分钟更新一次


        字段解释：
        - price 兑换成本
        - profit  与交易行的差值
        - purchaseCount  可兑换次数
        - purchaseDuration 兑换刷新小时
        - task  解锁兑换要求
        - isGet 是否交易行存在
        - is_pay 是否可上架交易行
        - sxf 手续费
        - priceMax 出售价格
        - priceProfit 出售利润
        - count 兑换数量
        - unlockLevel 解锁要求
      tags:
        - 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
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
                        grade:
                          type: integer
                        id:
                          type: integer
                        isGet:
                          type: integer
                        is_pay:
                          type: integer
                        name:
                          type: string
                        objectID:
                          type: integer
                        oid:
                          type: integer
                        pic:
                          type: string
                        price:
                          type: integer
                        priceMax:
                          type: integer
                        priceProfit:
                          type: integer
                        primaryClass:
                          type: string
                        profit:
                          type: integer
                        purchaseCount:
                          type: string
                        purchaseDuration:
                          type: string
                        secondClass:
                          type: string
                        secondClassCN:
                          type: string
                        sxf:
                          type: integer
                        task:
                          type: string
                        unlockLevel:
                          type: string
                      required:
                        - count
                        - grade
                        - id
                        - isGet
                        - is_pay
                        - name
                        - objectID
                        - oid
                        - pic
                        - price
                        - priceMax
                        - priceProfit
                        - primaryClass
                        - profit
                        - purchaseCount
                        - purchaseDuration
                        - secondClass
                        - secondClassCN
                        - sxf
                        - task
                        - unlockLevel
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - count: 180
                    grade: 4
                    id: 19
                    isGet: 1
                    is_pay: 1
                    name: 5.45x39mm BT
                    objectID: 37120400001
                    oid: 1050
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                    price: 201699
                    priceMax: 308880
                    priceProfit: 67027
                    primaryClass: ammo
                    profit: 107181
                    purchaseCount: 1次
                    purchaseDuration: 24小时
                    secondClass: ammo5.45x39
                    secondClassCN: 5.45x39mm
                    sxf: 40154
                    task: ''
                    unlockLevel: 24级解锁
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-438373595-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 军需处兑换 - 战备假账

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/exchange_zb:
    get:
      summary: 军需处兑换 - 战备假账
      deprecated: false
      description: |
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        更新频率：
        - 每10分钟更新一次


        核心
        - jiazhang  假账数值
        - 战备值 =  price + jiazhang

        字段解释：
        - price 兑换成本
        - profit  与交易行的差值
        - purchaseCount  可兑换次数
        - purchaseDuration 兑换刷新小时
        - task  解锁兑换要求
        - isGet 是否交易行存在
        - is_pay 是否可上架交易行
        - sxf 手续费
        - priceMax 出售价格
        - priceProfit 出售利润
        - count 兑换数量
        - unlockLevel 解锁要求
      tags:
        - 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
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
                        grade:
                          type: integer
                        id:
                          type: integer
                        isGet:
                          type: integer
                        is_pay:
                          type: integer
                        jiazhang:
                          type: integer
                        name:
                          type: string
                        objectID:
                          type: integer
                        oid:
                          type: integer
                        pic:
                          type: string
                        price:
                          type: integer
                        priceMax:
                          type: integer
                        priceProfit:
                          type: integer
                        primaryClass:
                          type: string
                        profit:
                          type: integer
                        purchaseCount:
                          type: string
                        purchaseDuration:
                          type: string
                        secondClass:
                          type: string
                        secondClassCN:
                          type: string
                        sxf:
                          type: integer
                        task:
                          type: string
                        unlockLevel:
                          type: string
                      required:
                        - count
                        - grade
                        - id
                        - isGet
                        - is_pay
                        - jiazhang
                        - name
                        - objectID
                        - oid
                        - pic
                        - price
                        - priceMax
                        - priceProfit
                        - primaryClass
                        - profit
                        - purchaseCount
                        - purchaseDuration
                        - secondClass
                        - secondClassCN
                        - sxf
                        - task
                        - unlockLevel
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - count: 1
                    grade: 1
                    id: 470
                    isGet: 1
                    is_pay: 0
                    jiazhang: 60699
                    name: QCQ171冲锋枪
                    objectID: 18020000011
                    oid: 667
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/object/18020000011.png
                    price: 49849
                    priceMax: 0
                    priceProfit: 0
                    primaryClass: gun
                    profit: 56144
                    purchaseCount: 1次
                    purchaseDuration: 48小时
                    secondClass: gunSMG
                    secondClassCN: 冲锋枪
                    sxf: 0
                    task: 任务解锁
                    unlockLevel: 20级解锁
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/军需处兑换 + 特勤处制造
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-438374900-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 今日钥匙卡低价预测

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/keys_day:
    get:
      summary: 今日钥匙卡低价预测
      deprecated: false
      description: |
        资源扣除逻辑：
        - 请求并数据正常【1次】资源


        更新频率：
        - 每天0点更新，之后数据均一致
      tags:
        - 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/钥匙卡分析
      parameters:
        - name: mid
          in: query
          description: 地图数据 1大坝 2航天 3长弓 4巴克什 5监狱
          required: true
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
                        data:
                          type: array
                          items:
                            type: object
                            properties:
                              grade:
                                type: integer
                              id:
                                type: integer
                              mid:
                                type: integer
                              name:
                                type: string
                              oid:
                                type: integer
                              pic:
                                type: string
                              price:
                                type: integer
                            required:
                              - grade
                              - id
                              - mid
                              - name
                              - oid
                              - pic
                              - price
                        hour:
                          type: integer
                      required:
                        - data
                        - hour
                required:
                  - code
                  - message
                  - msg
                  - data
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/钥匙卡分析
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-399845563-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 子弹自选包收益

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/ammo_pack:
    get:
      summary: 子弹自选包收益
      deprecated: false
      description: |
        资源扣除逻辑：
        - 请求并数据正常【1次】资源


        更新频率：
        - 
      tags:
        - 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/子弹分析
      parameters:
        - name: grade
          in: query
          description: 品质等级 0~6
          required: true
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
                    type: object
                    properties:
                      3级子弹自选包:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: number
                            grade:
                              type: integer
                            id:
                              type: integer
                            name:
                              type: string
                            num:
                              type: integer
                            pic:
                              type: string
                            price:
                              type: integer
                            sy_price:
                              type: integer
                          required:
                            - bl
                            - grade
                            - id
                            - name
                            - num
                            - pic
                            - price
                            - sy_price
                      4级子弹自选包:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: number
                            grade:
                              type: integer
                            id:
                              type: integer
                            name:
                              type: string
                            num:
                              type: integer
                            pic:
                              type: string
                            price:
                              type: integer
                            sy_price:
                              type: integer
                          required:
                            - bl
                            - grade
                            - id
                            - name
                            - num
                            - pic
                            - price
                            - sy_price
                      5级子弹自选包:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: number
                            grade:
                              type: integer
                            id:
                              type: integer
                            name:
                              type: string
                            num:
                              type: integer
                            pic:
                              type: string
                            price:
                              type: integer
                            sy_price:
                              type: integer
                          required:
                            - bl
                            - grade
                            - id
                            - name
                            - num
                            - pic
                            - price
                            - sy_price
                      通行证基础子弹自选包:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: number
                            grade:
                              type: integer
                            id:
                              type: integer
                            name:
                              type: string
                            num:
                              type: integer
                            pic:
                              type: string
                            price:
                              type: integer
                            sy_price:
                              type: integer
                          required:
                            - bl
                            - grade
                            - id
                            - name
                            - num
                            - pic
                            - price
                            - sy_price
                      通行证高级子弹自选包:
                        type: array
                        items:
                          type: object
                          properties:
                            bl:
                              type: number
                            grade:
                              type: integer
                            id:
                              type: integer
                            name:
                              type: string
                            num:
                              type: integer
                            pic:
                              type: string
                            price:
                              type: integer
                            sy_price:
                              type: integer
                          required:
                            - bl
                            - grade
                            - id
                            - name
                            - num
                            - pic
                            - price
                            - sy_price
                    required:
                      - 3级子弹自选包
                      - 4级子弹自选包
                      - 5级子弹自选包
                      - 通行证基础子弹自选包
                      - 通行证高级子弹自选包
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  3级子弹自选包:
                    - bl: 3.36
                      grade: 3
                      id: 45
                      name: .45 ACP FMJ
                      num: 180
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                      price: 461
                      sy_price: 82980
                    - bl: 0
                      grade: 3
                      id: 49
                      name: 5.7x28mm L191
                      num: 200
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                      price: 405
                      sy_price: 81000
                    - bl: 8.2
                      grade: 3
                      id: 42
                      name: 7.62x51mm BPZ
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                      price: 528
                      sy_price: 79200
                    - bl: 0
                      grade: 3
                      id: 36
                      name: 5.56x45mm M855
                      num: 200
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                      price: 391
                      sy_price: 78200
                    - bl: 4.01
                      grade: 3
                      id: 51
                      name: .357 Magnum JHP
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                      price: 519
                      sy_price: 77850
                  4级子弹自选包:
                    - bl: -9.31
                      grade: 4
                      id: 22
                      name: 6.8x51mm FMJ
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                      price: 1754
                      sy_price: 263100
                    - bl: -9.12
                      grade: 4
                      id: 20
                      name: 5.8x42mm DBP10
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                      price: 1743
                      sy_price: 261450
                    - bl: -10.41
                      grade: 4
                      id: 21
                      name: 9x39mm SP6
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                      price: 1721
                      sy_price: 258150
                    - bl: -8.84
                      grade: 4
                      id: 26
                      name: 7.62x54R LPS
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                      price: 1650
                      sy_price: 247500
                    - bl: -10.7
                      grade: 4
                      id: 29
                      name: 5.7x28mm SS193
                      num: 150
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                      price: 1503
                      sy_price: 225450
                  5级子弹自选包:
                    - bl: -7.52
                      grade: 5
                      id: 8
                      name: 5.8x42mm DVC12
                      num: 240
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                      price: 4305
                      sy_price: 1033200
                    - bl: -5.85
                      grade: 5
                      id: 14
                      name: 5.7x28mm SS190
                      num: 240
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                      price: 4182
                      sy_price: 1003680
                    - bl: -7.27
                      grade: 5
                      id: 5
                      name: 5.56x45mm M995
                      num: 240
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                      price: 4147
                      sy_price: 995280
                    - bl: -5.24
                      grade: 5
                      id: 7
                      name: 5.45x39mm BS
                      num: 240
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                      price: 3979
                      sy_price: 954960
                    - bl: -3.89
                      grade: 5
                      id: 10
                      name: 6.8x51mm Hybrid
                      num: 200
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                      price: 4694
                      sy_price: 938800
                  通行证基础子弹自选包:
                    - bl: 8.2
                      grade: 3
                      id: 42
                      name: 7.62x51mm BPZ
                      num: 90
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                      price: 528
                      sy_price: 47520
                    - bl: 3.36
                      grade: 3
                      id: 45
                      name: .45 ACP FMJ
                      num: 100
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                      price: 461
                      sy_price: 46100
                    - bl: 0
                      grade: 3
                      id: 49
                      name: 5.7x28mm L191
                      num: 110
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                      price: 405
                      sy_price: 44550
                    - bl: 0
                      grade: 3
                      id: 36
                      name: 5.56x45mm M855
                      num: 110
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                      price: 391
                      sy_price: 43010
                    - bl: -11.45
                      grade: 3
                      id: 40
                      name: 9x39mm SP5
                      num: 90
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                      price: 464
                      sy_price: 41760
                  通行证高级子弹自选包:
                    - bl: -9.12
                      grade: 4
                      id: 20
                      name: 5.8x42mm DBP10
                      num: 50
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                      price: 1743
                      sy_price: 87150
                    - bl: 0
                      grade: 4
                      id: 24
                      name: 12.7x55mm PD12双头弹
                      num: 25
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                      price: 3067
                      sy_price: 76675
                    - bl: -10.7
                      grade: 4
                      id: 29
                      name: 5.7x28mm SS193
                      num: 50
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                      price: 1503
                      sy_price: 75150
                    - bl: 12.07
                      grade: 4
                      id: 23
                      name: 12.7x55mm PS12
                      num: 30
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                      price: 2367
                      sy_price: 71010
                    - bl: -9.31
                      grade: 4
                      id: 22
                      name: 6.8x51mm FMJ
                      num: 40
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                      price: 1754
                      sy_price: 70160
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/子弹分析
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-399847075-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 今日倒子弹低价预测

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/ammo_day:
    get:
      summary: 今日倒子弹低价预测
      deprecated: false
      description: |
        资源扣除逻辑：
        - 请求并数据正常【1次】资源


        更新频率：
        - 
      tags:
        - 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/子弹分析
      parameters:
        - name: grade
          in: query
          description: 品质等级 0~6
          required: true
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
                        data:
                          type: array
                          items:
                            type: object
                            properties:
                              grade:
                                type: integer
                              id:
                                type: integer
                              name:
                                type: string
                              oid:
                                type: integer
                              pic:
                                type: string
                              price:
                                type: integer
                            required:
                              - grade
                              - id
                              - name
                              - oid
                              - pic
                              - price
                        hour:
                          type: integer
                      required:
                        - data
                        - hour
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  - data:
                      - grade: 5
                        id: 10
                        name: 6.8x51mm Hybrid
                        oid: 648
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 4694
                      - grade: 5
                        id: 12
                        name: 7.62x51mm M62
                        oid: 657
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 5467
                      - grade: 4
                        id: 21
                        name: 9x39mm SP6
                        oid: 667
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 1761
                      - grade: 4
                        id: 22
                        name: 6.8x51mm FMJ
                        oid: 647
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 1844
                      - grade: 4
                        id: 23
                        name: 12.7x55mm PS12
                        oid: 1070
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2112
                      - grade: 4
                        id: 25
                        name: 7.62x51mm M80
                        oid: 654
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 1298
                      - grade: 4
                        id: 30
                        name: .357 Magnum FMJ
                        oid: 608
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 455
                      - grade: 3
                        id: 39
                        name: 5.8x42mm DVP88
                        oid: 644
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 388
                      - grade: 3
                        id: 40
                        name: 9x39mm SP5
                        oid: 665
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 504
                      - grade: 3
                        id: 41
                        name: 12.7x55mm PS12A
                        oid: 625
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 589
                      - grade: 3
                        id: 47
                        name: 9x19mm AP6.3
                        oid: 662
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 396
                      - grade: 3
                        id: 48
                        name: 5.7x28mm R37.F
                        oid: 637
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 367
                      - grade: 3
                        id: 55
                        name: 12 Gauge独头 FTX
                        oid: 621
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 499
                      - grade: 3
                        id: 56
                        name: 4.6x30mm Subsonic SX
                        oid: 670
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 387
                      - grade: 4
                        id: 1117
                        name: 碳纤维刺骨箭矢
                        oid: 671
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270400001.png
                        price: 1574
                      - grade: 3
                        id: 1118
                        name: 玻纤柳叶箭矢
                        oid: 672
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270300001.png
                        price: 529
                      - grade: 5
                        id: 1282
                        name: 45-70 Govt FTX
                        oid: 1210
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290500001.png
                        price: 3275
                    hour: 0
                  - data:
                      - grade: 5
                        id: 10
                        name: 6.8x51mm Hybrid
                        oid: 648
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 4694
                      - grade: 4
                        id: 17
                        name: 5.56x45mm M855A1
                        oid: 636
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 1461
                      - grade: 4
                        id: 20
                        name: 5.8x42mm DBP10
                        oid: 643
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 1673
                      - grade: 4
                        id: 22
                        name: 6.8x51mm FMJ
                        oid: 647
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 1844
                      - grade: 4
                        id: 25
                        name: 7.62x51mm M80
                        oid: 654
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 1298
                      - grade: 4
                        id: 26
                        name: 7.62x54R LPS
                        oid: 659
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 1690
                      - grade: 4
                        id: 33
                        name: 12 Gauge 龙息弹
                        oid: 619
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 1920
                      - grade: 3
                        id: 37
                        name: 7.62x39mm PS
                        oid: 649
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 386
                      - grade: 3
                        id: 40
                        name: 9x39mm SP5
                        oid: 665
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 504
                      - grade: 3
                        id: 55
                        name: 12 Gauge独头 FTX
                        oid: 621
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 499
                      - grade: 5
                        id: 1282
                        name: 45-70 Govt FTX
                        oid: 1210
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290500001.png
                        price: 3275
                    hour: 1
                  - data:
                      - grade: 4
                        id: 17
                        name: 5.56x45mm M855A1
                        oid: 636
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 1461
                      - grade: 4
                        id: 20
                        name: 5.8x42mm DBP10
                        oid: 643
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 1673
                      - grade: 4
                        id: 21
                        name: 9x39mm SP6
                        oid: 667
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 1761
                      - grade: 4
                        id: 22
                        name: 6.8x51mm FMJ
                        oid: 647
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 1844
                      - grade: 4
                        id: 30
                        name: .357 Magnum FMJ
                        oid: 608
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 455
                      - grade: 4
                        id: 33
                        name: 12 Gauge 龙息弹
                        oid: 619
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 1920
                      - grade: 3
                        id: 40
                        name: 9x39mm SP5
                        oid: 665
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 504
                      - grade: 3
                        id: 48
                        name: 5.7x28mm R37.F
                        oid: 637
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 367
                      - grade: 3
                        id: 49
                        name: 5.7x28mm L191
                        oid: 642
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 405
                      - grade: 3
                        id: 51
                        name: .357 Magnum JHP
                        oid: 605
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 479
                      - grade: 3
                        id: 53
                        name: 12 Gauge 8.5毫米鹿弹
                        oid: 623
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 248
                      - grade: 3
                        id: 55
                        name: 12 Gauge独头 FTX
                        oid: 621
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 499
                      - grade: 5
                        id: 1282
                        name: 45-70 Govt FTX
                        oid: 1210
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290500001.png
                        price: 3275
                    hour: 2
                  - data:
                      - grade: 4
                        id: 17
                        name: 5.56x45mm M855A1
                        oid: 636
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 1461
                      - grade: 4
                        id: 20
                        name: 5.8x42mm DBP10
                        oid: 643
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 1673
                      - grade: 4
                        id: 21
                        name: 9x39mm SP6
                        oid: 667
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 1761
                      - grade: 3
                        id: 42
                        name: 7.62x51mm BPZ
                        oid: 655
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 508
                      - grade: 3
                        id: 43
                        name: 7.62x54R T46M
                        oid: 660
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 482
                      - grade: 3
                        id: 46
                        name: 9x19mm RIP
                        oid: 663
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 498
                      - grade: 3
                        id: 52
                        name: .50 AE JHP
                        oid: 615
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 484
                      - grade: 3
                        id: 53
                        name: 12 Gauge 8.5毫米鹿弹
                        oid: 623
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 248
                      - grade: 3
                        id: 57
                        name: .300BLK_3
                        oid: 1223
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 422
                    hour: 3
                  - data:
                      - grade: 4
                        id: 19
                        name: 5.45x39mm BT
                        oid: 630
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 841
                      - grade: 4
                        id: 23
                        name: 12.7x55mm PS12
                        oid: 1070
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2112
                      - grade: 4
                        id: 25
                        name: 7.62x51mm M80
                        oid: 654
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 1298
                      - grade: 4
                        id: 26
                        name: 7.62x54R LPS
                        oid: 659
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 1690
                      - grade: 4
                        id: 31
                        name: .50 AE FMJ
                        oid: 614
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 1100
                      - grade: 3
                        id: 38
                        name: 5.45x39mm PS
                        oid: 631
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 354
                      - grade: 3
                        id: 42
                        name: 7.62x51mm BPZ
                        oid: 655
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 508
                      - grade: 3
                        id: 43
                        name: 7.62x54R T46M
                        oid: 660
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 482
                      - grade: 3
                        id: 45
                        name: .45 ACP FMJ
                        oid: 612
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 446
                      - grade: 3
                        id: 49
                        name: 5.7x28mm L191
                        oid: 642
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 405
                      - grade: 3
                        id: 51
                        name: .357 Magnum JHP
                        oid: 605
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 479
                      - grade: 3
                        id: 52
                        name: .50 AE JHP
                        oid: 615
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 484
                      - grade: 3
                        id: 53
                        name: 12 Gauge 8.5毫米鹿弹
                        oid: 623
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 248
                      - grade: 3
                        id: 54
                        name: 12 Gauge 箭形弹
                        oid: 622
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 433
                      - grade: 3
                        id: 57
                        name: .300BLK_3
                        oid: 1223
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 422
                    hour: 4
                  - data:
                      - grade: 5
                        id: 11
                        name: 12.7x55mm PS12B
                        oid: 626
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 4963
                      - grade: 4
                        id: 19
                        name: 5.45x39mm BT
                        oid: 630
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 841
                      - grade: 4
                        id: 24
                        name: 12.7x55mm PD12双头弹
                        oid: 627
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2867
                      - grade: 4
                        id: 26
                        name: 7.62x54R LPS
                        oid: 659
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 1690
                      - grade: 4
                        id: 29
                        name: 5.7x28mm SS193
                        oid: 641
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 1413
                      - grade: 4
                        id: 32
                        name: 12 Gauge独头 AP-20
                        oid: 620
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 551
                      - grade: 3
                        id: 36
                        name: 5.56x45mm M855
                        oid: 1069
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 391
                      - grade: 3
                        id: 37
                        name: 7.62x39mm PS
                        oid: 649
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 386
                      - grade: 3
                        id: 38
                        name: 5.45x39mm PS
                        oid: 631
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 354
                      - grade: 3
                        id: 42
                        name: 7.62x51mm BPZ
                        oid: 655
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 508
                      - grade: 3
                        id: 43
                        name: 7.62x54R T46M
                        oid: 660
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 482
                      - grade: 3
                        id: 44
                        name: .45 ACP RIP
                        oid: 613
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 485
                      - grade: 3
                        id: 46
                        name: 9x19mm RIP
                        oid: 663
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 498
                      - grade: 3
                        id: 49
                        name: 5.7x28mm L191
                        oid: 642
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 405
                      - grade: 3
                        id: 52
                        name: .50 AE JHP
                        oid: 615
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 484
                      - grade: 3
                        id: 54
                        name: 12 Gauge 箭形弹
                        oid: 622
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 433
                      - grade: 4
                        id: 1117
                        name: 碳纤维刺骨箭矢
                        oid: 671
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270400001.png
                        price: 1574
                      - grade: 3
                        id: 1118
                        name: 玻纤柳叶箭矢
                        oid: 672
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270300001.png
                        price: 529
                      - grade: 3
                        id: 1284
                        name: 45-70 Govt RN
                        oid: 1212
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290300001.png
                        price: 554
                    hour: 5
                  - data:
                      - grade: 5
                        id: 13
                        name: 7.62x54R BT
                        oid: 658
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 4174
                      - grade: 4
                        id: 18
                        name: 7.62x39mm BP
                        oid: 653
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 1226
                      - grade: 4
                        id: 24
                        name: 12.7x55mm PD12双头弹
                        oid: 627
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2867
                      - grade: 4
                        id: 29
                        name: 5.7x28mm SS193
                        oid: 641
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 1413
                      - grade: 4
                        id: 32
                        name: 12 Gauge独头 AP-20
                        oid: 620
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 551
                      - grade: 4
                        id: 34
                        name: 4.6x30mm FMJ SX
                        oid: 669
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 1070
                      - grade: 4
                        id: 35
                        name: .300BLK_4
                        oid: 1222
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 1547
                      - grade: 3
                        id: 36
                        name: 5.56x45mm M855
                        oid: 1069
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 391
                      - grade: 3
                        id: 38
                        name: 5.45x39mm PS
                        oid: 631
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 354
                      - grade: 3
                        id: 41
                        name: 12.7x55mm PS12A
                        oid: 625
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 589
                      - grade: 3
                        id: 44
                        name: .45 ACP RIP
                        oid: 613
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 485
                      - grade: 3
                        id: 45
                        name: .45 ACP FMJ
                        oid: 612
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 446
                      - grade: 3
                        id: 46
                        name: 9x19mm RIP
                        oid: 663
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 498
                      - grade: 3
                        id: 50
                        name: .357 Magnum 9号霰射"鼠弹"
                        oid: 606
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 133
                      - grade: 3
                        id: 51
                        name: .357 Magnum JHP
                        oid: 605
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 479
                      - grade: 4
                        id: 1283
                        name: 45-70 Govt FMJ
                        oid: 1211
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290400001.png
                        price: 1668
                    hour: 6
                  - data:
                      - grade: 5
                        id: 8
                        name: 5.8x42mm DVC12
                        oid: 646
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 4235
                      - grade: 5
                        id: 9
                        name: 9x39mm BP
                        oid: 666
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 4289
                      - grade: 5
                        id: 11
                        name: 12.7x55mm PS12B
                        oid: 626
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 4963
                      - grade: 5
                        id: 12
                        name: 7.62x51mm M62
                        oid: 657
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 5467
                      - grade: 5
                        id: 14
                        name: 5.7x28mm SS190
                        oid: 640
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 4052
                      - grade: 4
                        id: 18
                        name: 7.62x39mm BP
                        oid: 653
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 1226
                      - grade: 4
                        id: 24
                        name: 12.7x55mm PD12双头弹
                        oid: 627
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2867
                      - grade: 4
                        id: 27
                        name: .45 ACP AP
                        oid: 611
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 1225
                      - grade: 4
                        id: 28
                        name: 9x19mm PBP
                        oid: 661
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 976
                      - grade: 4
                        id: 33
                        name: 12 Gauge 龙息弹
                        oid: 619
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 1920
                      - grade: 4
                        id: 34
                        name: 4.6x30mm FMJ SX
                        oid: 669
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 1070
                      - grade: 4
                        id: 35
                        name: .300BLK_4
                        oid: 1222
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 1547
                      - grade: 3
                        id: 44
                        name: .45 ACP RIP
                        oid: 613
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 485
                      - grade: 5
                        id: 1116
                        name: 碳纤维穿甲箭矢
                        oid: 673
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270500001.png
                        price: 5069
                      - grade: 4
                        id: 1117
                        name: 碳纤维刺骨箭矢
                        oid: 671
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270400001.png
                        price: 1574
                    hour: 7
                  - data:
                      - grade: 5
                        id: 5
                        name: 5.56x45mm M995
                        oid: 635
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 4082
                      - grade: 5
                        id: 6
                        name: 7.62x39mm AP
                        oid: 652
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 4371
                      - grade: 5
                        id: 7
                        name: 5.45x39mm BS
                        oid: 629
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 3924
                      - grade: 5
                        id: 8
                        name: 5.8x42mm DVC12
                        oid: 646
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 4235
                      - grade: 5
                        id: 9
                        name: 9x39mm BP
                        oid: 666
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 4289
                      - grade: 5
                        id: 10
                        name: 6.8x51mm Hybrid
                        oid: 648
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/6.8x51mm.png
                        price: 4694
                      - grade: 5
                        id: 12
                        name: 7.62x51mm M62
                        oid: 657
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x51mm.png
                        price: 5467
                      - grade: 5
                        id: 14
                        name: 5.7x28mm SS190
                        oid: 640
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 4052
                      - grade: 5
                        id: 15
                        name: 4.6x30mm AP SX
                        oid: 668
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 3447
                      - grade: 5
                        id: 16
                        name: .300BLK_5
                        oid: 1221
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 4218
                      - grade: 4
                        id: 18
                        name: 7.62x39mm BP
                        oid: 653
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 1226
                      - grade: 4
                        id: 29
                        name: 5.7x28mm SS193
                        oid: 641
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 1413
                      - grade: 4
                        id: 35
                        name: .300BLK_4
                        oid: 1222
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 1547
                      - grade: 5
                        id: 1116
                        name: 碳纤维穿甲箭矢
                        oid: 673
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270500001.png
                        price: 5069
                    hour: 8
                  - data:
                      - grade: 5
                        id: 8
                        name: 5.8x42mm DVC12
                        oid: 646
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.8x42mm.png
                        price: 4235
                      - grade: 5
                        id: 16
                        name: .300BLK_5
                        oid: 1221
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 4218
                      - grade: 5
                        id: 1116
                        name: 碳纤维穿甲箭矢
                        oid: 673
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270500001.png
                        price: 5069
                    hour: 9
                  - data:
                      - grade: 5
                        id: 5
                        name: 5.56x45mm M995
                        oid: 635
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 4082
                      - grade: 5
                        id: 6
                        name: 7.62x39mm AP
                        oid: 652
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 4371
                      - grade: 5
                        id: 16
                        name: .300BLK_5
                        oid: 1221
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 4218
                    hour: 10
                  - data:
                      - grade: 5
                        id: 5
                        name: 5.56x45mm M995
                        oid: 635
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 4082
                      - grade: 5
                        id: 6
                        name: 7.62x39mm AP
                        oid: 652
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 4371
                      - grade: 5
                        id: 7
                        name: 5.45x39mm BS
                        oid: 629
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 3924
                      - grade: 5
                        id: 15
                        name: 4.6x30mm AP SX
                        oid: 668
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 3447
                      - grade: 3
                        id: 47
                        name: 9x19mm AP6.3
                        oid: 662
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 396
                    hour: 11
                  - data:
                      - grade: 5
                        id: 7
                        name: 5.45x39mm BS
                        oid: 629
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 3924
                      - grade: 5
                        id: 15
                        name: 4.6x30mm AP SX
                        oid: 668
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 3447
                    hour: 12
                  - data:
                      - grade: 5
                        id: 13
                        name: 7.62x54R BT
                        oid: 658
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 4174
                    hour: 13
                  - data:
                      - grade: 5
                        id: 9
                        name: 9x39mm BP
                        oid: 666
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x39mm.png
                        price: 4289
                    hour: 14
                  - data: []
                    hour: 15
                  - data: []
                    hour: 16
                  - data:
                      - grade: 3
                        id: 47
                        name: 9x19mm AP6.3
                        oid: 662
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 396
                    hour: 17
                  - data:
                      - grade: 5
                        id: 11
                        name: 12.7x55mm PS12B
                        oid: 626
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 4963
                      - grade: 5
                        id: 14
                        name: 5.7x28mm SS190
                        oid: 640
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 4052
                      - grade: 3
                        id: 57
                        name: .300BLK_3
                        oid: 1223
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/300BLK.png
                        price: 422
                    hour: 18
                  - data:
                      - grade: 4
                        id: 27
                        name: .45 ACP AP
                        oid: 611
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 1225
                      - grade: 4
                        id: 28
                        name: 9x19mm PBP
                        oid: 661
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 976
                    hour: 19
                  - data:
                      - grade: 4
                        id: 19
                        name: 5.45x39mm BT
                        oid: 630
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.45x39mm.png
                        price: 841
                      - grade: 4
                        id: 27
                        name: .45 ACP AP
                        oid: 611
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 1225
                      - grade: 3
                        id: 50
                        name: .357 Magnum 9号霰射"鼠弹"
                        oid: 606
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 133
                    hour: 20
                  - data:
                      - grade: 4
                        id: 31
                        name: .50 AE FMJ
                        oid: 614
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 1100
                      - grade: 3
                        id: 50
                        name: .357 Magnum 9号霰射"鼠弹"
                        oid: 606
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 133
                    hour: 21
                  - data:
                      - grade: 4
                        id: 28
                        name: 9x19mm PBP
                        oid: 661
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/9x19mm.png
                        price: 976
                      - grade: 4
                        id: 32
                        name: 12 Gauge独头 AP-20
                        oid: 620
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 551
                      - grade: 3
                        id: 36
                        name: 5.56x45mm M855
                        oid: 1069
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.56x45mm.png
                        price: 391
                      - grade: 3
                        id: 41
                        name: 12.7x55mm PS12A
                        oid: 625
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 589
                      - grade: 3
                        id: 1118
                        name: 玻纤柳叶箭矢
                        oid: 672
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37270300001.png
                        price: 529
                      - grade: 4
                        id: 1283
                        name: 45-70 Govt FMJ
                        oid: 1211
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290400001.png
                        price: 1668
                      - grade: 3
                        id: 1284
                        name: 45-70 Govt RN
                        oid: 1212
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290300001.png
                        price: 554
                    hour: 22
                  - data:
                      - grade: 5
                        id: 13
                        name: 7.62x54R BT
                        oid: 658
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x54mm.png
                        price: 4174
                      - grade: 4
                        id: 23
                        name: 12.7x55mm PS12
                        oid: 1070
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12.7x55mm.png
                        price: 2112
                      - grade: 4
                        id: 30
                        name: .357 Magnum FMJ
                        oid: 608
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/357-Magnum.png
                        price: 455
                      - grade: 4
                        id: 31
                        name: .50 AE FMJ
                        oid: 614
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/50-AE.png
                        price: 1100
                      - grade: 4
                        id: 34
                        name: 4.6x30mm FMJ SX
                        oid: 669
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/4.6x30mm.png
                        price: 1070
                      - grade: 3
                        id: 37
                        name: 7.62x39mm PS
                        oid: 649
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/7.62x39mm.png
                        price: 386
                      - grade: 3
                        id: 45
                        name: .45 ACP FMJ
                        oid: 612
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/45-ACP.png
                        price: 446
                      - grade: 3
                        id: 48
                        name: 5.7x28mm R37.F
                        oid: 637
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/5.7x28mm.png
                        price: 367
                      - grade: 3
                        id: 54
                        name: 12 Gauge 箭形弹
                        oid: 622
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/gun/ammo/12-Gauge.png
                        price: 433
                      - grade: 4
                        id: 1283
                        name: 45-70 Govt FMJ
                        oid: 1211
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290400001.png
                        price: 1668
                      - grade: 3
                        id: 1284
                        name: 45-70 Govt RN
                        oid: 1212
                        pic: >-
                          https://playerhub.df.qq.com/playerhub/60004/object/37290300001.png
                        price: 554
                    hour: 23
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/钥匙/子弹 数据分析/子弹分析
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-399848912-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 改枪码 - 烽火

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/gun_gqm:
    get:
      summary: 改枪码 - 烽火
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【1次】资源

        ----
        字段需注意：
        - 【oid】 代表的是【第三代数据ID】（一般其他接口请求这个）
        - 【iid】 为【第二代数据ID】，目前已不适用，记录对照即可。
        - 【objectID】为【三角洲官方小程序】的数据ID，仅作为数据对应。


        ----
        【mtype】字段需注意，允许的值：
        - 步枪
        - 冲锋枪
        - 精确射手步枪
        - 狙击步枪
        - 轻机枪
        - 霰弹枪
        - 手枪
        - 特殊武器
      tags:
        - 三角洲数据帝 - 基础接口/改枪码
      parameters:
        - name: p
          in: query
          description: 页码
          required: false
          example: 3
          schema:
            type: integer
        - name: mtype
          in: query
          description: 枪械分类
          required: false
          schema:
            type: string
        - name: gun
          in: query
          description: 枪械名称
          required: false
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
                  msg:
                    type: string
                  count:
                    type: integer
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: integer
                        tid:
                          type: integer
                        iid:
                          type: integer
                        oid:
                          type: integer
                        objectID:
                          type: integer
                        objectName:
                          type: string
                        name:
                          type: string
                        solutionCode:
                          type: string
                        pic:
                          type: string
                        authorNickname:
                          type: string
                        price:
                          type: integer
                        pjID:
                          type: string
                        likeNum:
                          type: integer
                        applyNum:
                          type: integer
                        accessory:
                          type: string
                        propertiesDetail:
                          type: string
                        accObjectEffect:
                          type: string
                        solutionType:
                          type: string
                        m_type:
                          type: string
                        properties_data:
                          type: array
                          items:
                            type: object
                            properties:
                              change:
                                type: string
                              color:
                                type: string
                              name:
                                type: string
                              value:
                                type: integer
                            required:
                              - change
                              - color
                              - name
                              - value
                      required:
                        - id
                        - tid
                        - iid
                        - oid
                        - objectID
                        - objectName
                        - name
                        - solutionCode
                        - pic
                        - authorNickname
                        - price
                        - pjID
                        - likeNum
                        - applyNum
                        - accessory
                        - propertiesDetail
                        - accObjectEffect
                        - solutionType
                        - m_type
                        - properties_data
                required:
                  - code
                  - msg
                  - count
                  - data
              example:
                code: 0
                msg: 成功
                count: 395
                data:
                  - id: 328
                    iid: 1303
                    oid: 1232
                    objectID: 18020000012
                    objectName: MK4冲锋枪
                    name: 52w满改MK4
                    solutionCode: MK4冲锋枪-烽火地带-6I5GFAS0B4CK4I22HLEI9
                    pic: >-
                      https://playerhub.df.qq.com/playerhub/60004/251114/7f79314f-bd51-4859-9cdb-bfb83bf470fc.png
                    authorNickname: 千岁Sui
                    propertiesDetail: >-
                      {"origin":{"meatHarm":34,"shootDistance":20,"recoil":46,"control":60,"stable":51,"hipShot":58,"armorHarm":30,"fireSpeed":793},"change":{"meatHarm":0,"shootDistance":6,"recoil":24,"control":-10,"stable":-4,"hipShot":-16,"armorHarm":0,"fireSpeed":0}}
                    solutionType: ''
                    m_type: 冲锋枪
                    properties_data:
                      - change: '-'
                        color: ''
                        name: 肉伤
                        value: 34
                      - change: '-'
                        color: ''
                        name: 甲伤
                        value: 30
                      - change: '-'
                        color: ''
                        name: 射速
                        value: 793
                      - change: '+6'
                        color: has-text-success
                        name: 射程
                        value: 26
                      - change: '+24'
                        color: has-text-success
                        name: 后座
                        value: 70
                      - change: '-10'
                        color: has-text-danger
                        name: 操控
                        value: 50
                      - change: '-4'
                        color: has-text-danger
                        name: 稳定性
                        value: 47
                      - change: '-16'
                        color: has-text-danger
                        name: 腰射
                        value: 42
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/改枪码
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-408884672-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 赛季任务- S11

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/task_s11:
    get:
      summary: 赛季任务- S11
      deprecated: false
      description: |-
        资源扣除逻辑：
        - 请求并数据正常【10次】资源
      tags:
        - 三角洲数据帝 - 基础接口/赛季任务
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
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: array
                    items:
                      type: object
                      properties:
                        GropStarCount:
                          type: string
                        id:
                          type: integer
                        info:
                          type: string
                        jd:
                          type: string
                        lx:
                          type: string
                        name:
                          type: string
                        rewards:
                          type: string
                        task:
                          type: string
                        yq:
                          type: string
                      required:
                        - GropStarCount
                        - id
                        - info
                        - jd
                        - lx
                        - name
                        - rewards
                        - task
                        - yq
                required:
                  - code
                  - message
                  - msg
                  - data
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口/赛季任务
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-513199366-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 今日密码

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/map_pwd:
    get:
      summary: 今日密码
      deprecated: false
      description: |-

        资源扣除逻辑：
        - 请求并数据正常【1次】资源
        - 数据异常【0次】资源


        每天凌晨 0点5分 更新，之后数据均一致。
        - a 零号大坝
        - b  长弓溪谷
        - c  巴克什
        - d  航天基地
        - e 潮汐监狱
        - f  AZ3

        值为 - 为没更新
      tags:
        - 三角洲数据帝 - 基础接口
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
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      a:
                        type: array
                        items:
                          type: string
                      b:
                        type: array
                        items:
                          type: string
                      c:
                        type: array
                        items:
                          type: string
                      d:
                        type: array
                        items:
                          type: string
                      e:
                        type: array
                        items:
                          type: string
                    required:
                      - a
                      - b
                      - c
                      - d
                      - e
                required:
                  - code
                  - message
                  - msg
                  - data
              example:
                code: 0
                message: 成功
                msg: 成功
                data:
                  a:
                    - '0639'
                    - '2025-12-26'
                  b:
                    - '4842'
                    - '2025-12-26'
                  c:
                    - '8577'
                    - '2025-12-26'
                  d:
                    - '0013'
                    - '2025-12-26'
                  e:
                    - '8322'
                    - '2025-12-26'
                  f:
                    - '1111'
                    - '2025-12-26'
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-396183648-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```
# 当前市集所需物品

## OpenAPI Specification

```yaml
openapi: 3.0.1
info:
  title: ''
  description: ''
  version: 1.0.0
paths:
  /v1/sjz_api/market:
    get:
      summary: 当前市集所需物品
      deprecated: false
      description: |

        资源扣除逻辑：
        - 请求并数据正常【1次】资源
        - 数据异常【0次】资源
      tags:
        - 三角洲数据帝 - 基础接口
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
                  message:
                    type: string
                  msg:
                    type: string
                  data:
                    type: object
                    properties:
                      items:
                        type: array
                        items:
                          type: object
                          properties:
                            id:
                              type: integer
                              title: 第三代ID
                            name:
                              type: string
                              title: 名字
                            pic:
                              type: string
                              title: 图标
                            grade:
                              type: integer
                              title: 等级
                          required:
                            - id
                            - name
                            - pic
                            - grade
                          x-apifox-orders:
                            - id
                            - name
                            - pic
                            - grade
                      begin_date:
                        type: string
                        title: 开始日期
                      end_date:
                        type: string
                        title: 结束日期
                      begin_time:
                        type: integer
                        title: 开始时间戳
                      end_time:
                        type: integer
                        title: 结束时间戳
                    required:
                      - items
                      - begin_date
                      - end_date
                      - begin_time
                      - end_time
                    x-apifox-orders:
                      - items
                      - begin_date
                      - end_date
                      - begin_time
                      - end_time
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
                  items:
                    - id: 926
                      name: 电火机
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/15010010013.png
                      grade: 2
                    - id: 1010
                      name: 燃气罐
                      pic: >-
                        https://playerhub.df.qq.com/playerhub/60004/object/15020010037.png
                      grade: 3
                  begin_date: '2026-07-03'
                  end_date: '2026-07-10'
                  begin_time: 1783044000
                  end_time: 1783648800
          headers: {}
          x-apifox-name: 成功
      security: []
      x-apifox-folder: 三角洲数据帝 - 基础接口
      x-apifox-status: released
      x-run-in-apifox: https://app.apifox.com/web/project/7610373/apis/api-482045725-run
components:
  schemas: {}
  securitySchemes: {}
servers:
  - url: https://orzice.com/workApi
    description: 正式环境
security: []

```