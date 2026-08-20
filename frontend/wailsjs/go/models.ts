export namespace license {
	
	export class AuthState {
	    valid: boolean;
	    license_id?: string;
	    // Go type: time
	    expires_at?: any;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new AuthState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.license_id = source["license_id"];
	        this.expires_at = this.convertValues(source["expires_at"], null);
	        this.error = source["error"];
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

export namespace main {
	
	export class CheckConfig {
	    cookiesDir: string;
	    comboPath: string;
	    ulpPath: string;
	    proxyFile: string;
	    workers: number;
	    timeoutSec: number;
	    doHypixel: boolean;
	    doDonut: boolean;
	    doRename: boolean;
	    skinFile: string;
	    skinVariant: string;
	    allFiles: boolean;
	    noCookies: boolean;
	    noCreds: boolean;
	    ulpAll: boolean;
	    relocate: string;
	    runName: string;
	    discordWebhook: string;
	    doUpload: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CheckConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cookiesDir = source["cookiesDir"];
	        this.comboPath = source["comboPath"];
	        this.ulpPath = source["ulpPath"];
	        this.proxyFile = source["proxyFile"];
	        this.workers = source["workers"];
	        this.timeoutSec = source["timeoutSec"];
	        this.doHypixel = source["doHypixel"];
	        this.doDonut = source["doDonut"];
	        this.doRename = source["doRename"];
	        this.skinFile = source["skinFile"];
	        this.skinVariant = source["skinVariant"];
	        this.allFiles = source["allFiles"];
	        this.noCookies = source["noCookies"];
	        this.noCreds = source["noCreds"];
	        this.ulpAll = source["ulpAll"];
	        this.relocate = source["relocate"];
	        this.runName = source["runName"];
	        this.discordWebhook = source["discordWebhook"];
	        this.doUpload = source["doUpload"];
	    }
	}
	export class SystemStatus {
	    status: string;
	    uptime: string;
	    lastChecked: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.uptime = source["uptime"];
	        this.lastChecked = source["lastChecked"];
	    }
	}
	export class TelegramStatus {
	    configured: boolean;
	    loggedIn: boolean;
	    monitoring: boolean;
	    user: string;
	    phone: string;

	    static createFrom(source: any = {}) {
	        return new TelegramStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.loggedIn = source["loggedIn"];
	        this.monitoring = source["monitoring"];
	        this.user = source["user"];
	        this.phone = source["phone"];
	    }
	}

}

export namespace telegramauto {

	export class Channel {
	    id: number;
	    accessHash: number;
	    title: string;
	    username: string;
	    broadcast: boolean;
	    megagroup: boolean;

	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accessHash = source["accessHash"];
	        this.title = source["title"];
	        this.username = source["username"];
	        this.broadcast = source["broadcast"];
	        this.megagroup = source["megagroup"];
	    }
	}

	export class MonitorTarget {
	    id: number;
	    accessHash: number;
	    title: string;
	    broadcast: boolean;

	    static createFrom(source: any = {}) {
	        return new MonitorTarget(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.accessHash = source["accessHash"];
	        this.title = source["title"];
	        this.broadcast = source["broadcast"];
	    }
	}

	export class Config {
	    channels: MonitorTarget[];
	    keywords: string[];

	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.channels = this.convertValues(source["channels"], MonitorTarget);
	        this.keywords = source["keywords"];
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
