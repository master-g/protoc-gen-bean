package com.acme.model.vo.hello;

// Code generated by protoc-gen-bean. DO NOT EDIT.
// 2018-03-14 Wed 16:51:09 UTC+0800
//
//     hello.proto
//

import com.acme.model.vo.common.AnotherMsg;
import com.acme.model.vo.hello.Hello;

import java.util.List;

public class World {
    public List<Hello> hello;
    public List<AnotherMsg> elements;
    
    @Override
    public String toString() {
        return "World{" +
                "hello=" + hello + ","
                "elements=" + elements + "}"
    }
}
