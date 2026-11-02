export namespace core {
	
	export class Check {
	    name: string;
	    ok: boolean;
	    done: boolean;
	    ms: number;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.done = source["done"];
	        this.ms = source["ms"];
	    }
	}
	export class Settings {
	    ipv4Only: boolean;
	    autoConnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ipv4Only = source["ipv4Only"];
	        this.autoConnect = source["autoConnect"];
	    }
	}
	export class Status {
	    state: string;
	    since: number;
	    message: string;
	    pinned: number;
	    checks: Check[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.since = source["since"];
	        this.message = source["message"];
	        this.pinned = source["pinned"];
	        this.checks = this.convertValues(source["checks"], Check);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace logx {
	
	export class Entry {
	    // Go type: time
	    time: any;
	    level: string;
	    msg: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
	        this.msg = source["msg"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace stats {
	
	export class Service {
	    name: string;
	    active: number;
	    ok: number;
	    failed: number;
	    bytesIn: number;
	    bytesOut: number;
	    lastOk: number;
	    lastFail: number;
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.active = source["active"];
	        this.ok = source["ok"];
	        this.failed = source["failed"];
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	        this.lastOk = source["lastOk"];
	        this.lastFail = source["lastFail"];
	    }
	}
	export class Snapshot {
	    bytesIn: number;
	    bytesOut: number;
	    speedIn: number;
	    speedOut: number;
	    connections: number;
	    total: number;
	    failed: number;
	    services: Service[];
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	        this.speedIn = source["speedIn"];
	        this.speedOut = source["speedOut"];
	        this.connections = source["connections"];
	        this.total = source["total"];
	        this.failed = source["failed"];
	        this.services = this.convertValues(source["services"], Service);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

