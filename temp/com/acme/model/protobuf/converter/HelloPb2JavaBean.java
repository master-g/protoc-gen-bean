package com.acme.model.protobuf.converter;

// Code generated by protoc-gen-bean. DO NOT EDIT.
// 2018-03-13 Tue 19:54:26 UTC+0800
//
//     hello.proto
//

import com.acme.model.protobuf.PbHello;
import com.acme.model.vo.common.RspHead;
import com.acme.model.vo.hello.Greeting;
import com.acme.model.vo.hello.Hello;
import com.google.protobuf.InvalidProtocolBufferException;

import java.util.ArrayList;
import java.util.List;

public class HelloPb2JavaBean {
    public static Hello toHello(byte[] data) {
        try {
            PbHello.Hello pbHello = PbHello.Hello.parseFrom(data);
            
            Hello hello = new Hello();
            hello.header = CommonPb2JavaBean.toHeader(pbHello.getHeader());
            hello.greeting = pbHello.getGreeting();
            hello.name = pbHello.getName();
            hello.sig = pbHello.getSig();
            
            return hello;
        } catch (InvalidProtocolBufferException e) {
            e.printStackTrace();
        }
        
        return null;
    }
    
    public static World toWorld(byte[] data) {
        try {
            PbHello.World pbWorld = PbHello.World.parseFrom(data);
            
            World world = new World();
            world.hello = new ArrayList<>();
            world.hello = HelloPb2JavaBean.toHello(pbWorld.getHello());
            
            return world;
        } catch (InvalidProtocolBufferException e) {
            e.printStackTrace();
        }
        
        return null;
    }
    
}
