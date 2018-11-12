# protoc-gen-bean

Java Value Object Generator Plugin For Protobuf

## Requirement

Download and install the protocol buffer compiler.

you can find it here

[Github](https://github.com/google/protobuf)

or here

[Google](https://developers.google.com/protocol-buffers/)

## Installation

After install protocol buffer compiler, you can build protoc-gen-bean or download pre-built binary from [release](https://github.com/master-g/protoc-gen-bean/releases) page

## Usage

    protoc --plugin=protoc-gen-bean --bean_out=. *.proto
    
### Parameters

To pass extra parameters to the plugin, use a comma-separated parameter list separated from the output directory by a colon:

    protoc --plugin=protoc-gen-bean --bean_out=vopackage=vo,cvtpackage=protobuf.converter:. *.proto
    
* `vopkg=xxx` - java value object package
* `cvtpkg=xxx` - converter package

Consider file test.proto, containing

```proto
package proto.common;
option java_package = "com.acme.model.protobuf";
option java_outer_classname = "PbCommon";

message Hello {
  optional string msg = 1;
  optional int32 code = 2;
}
```

To create and play with a Test object from the example package,

**Java Value Object**

```java
package com.acme.model.vo.common;

public class Hello {
    public String msg;
    public int code;
    
    @Override
    public String toString() {
        return "Hello{" +
                "msg='" + msg + '\'' + ","
                "code=" + code + "}"
    }
}
```

**Object Converter**

```java
package com.acme.model.protobuf.converter;

import com.acme.model.protobuf.PbCommon;
import com.acme.model.vo.common.Hello;
import com.google.protobuf.InvalidProtocolBufferException;

public class CommonPb2JavaBean {
    public static Hello toHello(byte[] data) {
        try {
            PbCommon.Hello pbHello = PbCommon.Hello.parseFrom(data);
            
            Hello hello = new Hello();
            hello.msg = pbHello.getMsg();
            hello.code = pbHello.getCode();
            
            return hello;
        } catch (InvalidProtocolBufferException e) {
            e.printStackTrace();
        }
        
        return null;
    }
}
```

**Output Package Structure**

```
.
└── com
    └── acme
        └── model
            ├── protobuf
            │   └── converter
            │       ├── CommonJavaBean2Pb.java
            │       └── CommonPb2JavaBean.java
            └── vo
                └── common
                    └── Hello.java
```
