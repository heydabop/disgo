--
-- PostgreSQL database dump
--

-- Dumped from database version 12.2 (Debian 12.2-4)
-- Dumped by pg_dump version 12.2 (Debian 12.2-4)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- Name: on_record_update(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.on_record_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ begin new.update_date := now(); return new; end; $$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: discord_quote; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.discord_quote (
    id integer NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30),
    content text,
    score integer NOT NULL,
    is_fresh boolean NOT NULL
);


--
-- Name: discord_quote_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.discord_quote_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discord_quote_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.discord_quote_id_seq OWNED BY public.discord_quote.id;


--
-- Name: error; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.error (
    id uuid DEFAULT public.gen_random_uuid() NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    command text NOT NULL,
    args text,
    error text,
    reported_count integer DEFAULT 0 NOT NULL
);


--
-- Name: error_ip; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.error_ip (
    error_id uuid NOT NULL,
    ip inet NOT NULL
);


--
-- Name: message; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message (
    id numeric NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    chan_id numeric NOT NULL,
    author_id numeric NOT NULL,
    content text,
    update_date timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: own_username; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.own_username (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    author_id character varying(30) NOT NULL,
    username character varying(32) NOT NULL,
    locked_minutes integer NOT NULL,
    guild_id character varying(30)
);


--
-- Name: own_username_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.own_username_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: own_username_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.own_username_id_seq OWNED BY public.own_username.id;


--
-- Name: pee_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pee_log (
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    user_id character varying(30)
);


--
-- Name: reminder; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reminder (
    id integer NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30) NOT NULL,
    send_time timestamp with time zone NOT NULL,
    content text
);


--
-- Name: reminder_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.reminder_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reminder_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.reminder_id_seq OWNED BY public.reminder.id;


--
-- Name: shipment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shipment (
    id integer NOT NULL,
    carrier text NOT NULL,
    tracking_number text NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30) NOT NULL
);


--
-- Name: shipment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.shipment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shipment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.shipment_id_seq OWNED BY public.shipment.id;


--
-- Name: user_karma; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_karma (
    guild_id character varying(30) NOT NULL,
    user_id character varying(30) NOT NULL,
    karma integer NOT NULL
);


--
-- Name: user_money; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_money (
    guild_id character varying(30) NOT NULL,
    user_id character varying(30) NOT NULL,
    money double precision NOT NULL
);


--
-- Name: user_presence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_presence (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id numeric NOT NULL,
    user_id numeric NOT NULL,
    presence character varying(20) NOT NULL,
    game text
);


--
-- Name: user_presence_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_presence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_presence_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_presence_id_seq OWNED BY public.user_presence.id;


--
-- Name: voice_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.voice_state (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id character varying(30) NOT NULL,
    chan_id character varying(30),
    user_id character varying(30) NOT NULL,
    session_id character varying(60) NOT NULL,
    deaf boolean NOT NULL,
    mute boolean NOT NULL,
    self_deaf boolean NOT NULL,
    self_mute boolean NOT NULL,
    suppress boolean NOT NULL
);


--
-- Name: voice_state_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.voice_state_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: voice_state_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.voice_state_id_seq OWNED BY public.voice_state.id;


--
-- Name: vote; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vote (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id character varying(30) NOT NULL,
    message_id bigint NOT NULL,
    voter_id character varying(30) NOT NULL,
    votee_id character varying(30) NOT NULL,
    is_upvote boolean NOT NULL
);


--
-- Name: vote_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.vote_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vote_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.vote_id_seq OWNED BY public.vote.id;


--
-- Name: discord_quote id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discord_quote ALTER COLUMN id SET DEFAULT nextval('public.discord_quote_id_seq'::regclass);


--
-- Name: own_username id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.own_username ALTER COLUMN id SET DEFAULT nextval('public.own_username_id_seq'::regclass);


--
-- Name: reminder id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder ALTER COLUMN id SET DEFAULT nextval('public.reminder_id_seq'::regclass);


--
-- Name: shipment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shipment ALTER COLUMN id SET DEFAULT nextval('public.shipment_id_seq'::regclass);


--
-- Name: user_presence id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_presence ALTER COLUMN id SET DEFAULT nextval('public.user_presence_id_seq'::regclass);


--
-- Name: voice_state id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voice_state ALTER COLUMN id SET DEFAULT nextval('public.voice_state_id_seq'::regclass);


--
-- Name: vote id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vote ALTER COLUMN id SET DEFAULT nextval('public.vote_id_seq'::regclass);


--
-- Name: discord_quote discord_quote_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discord_quote
    ADD CONSTRAINT discord_quote_pkey PRIMARY KEY (id);


--
-- Name: error_ip error_ip_error_id_ip_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.error_ip
    ADD CONSTRAINT error_ip_error_id_ip_key UNIQUE (error_id, ip);


--
-- Name: error error_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.error
    ADD CONSTRAINT error_pkey PRIMARY KEY (id);


--
-- Name: message message_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message
    ADD CONSTRAINT message_pkey PRIMARY KEY (id);


--
-- Name: own_username own_username_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.own_username
    ADD CONSTRAINT own_username_pkey PRIMARY KEY (id);


--
-- Name: pee_log pee_log_create_date_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pee_log
    ADD CONSTRAINT pee_log_create_date_user_id_key UNIQUE (create_date, user_id);


--
-- Name: reminder reminder_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reminder
    ADD CONSTRAINT reminder_pkey PRIMARY KEY (id);


--
-- Name: shipment shipment_carrier_tracking_number_author_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shipment
    ADD CONSTRAINT shipment_carrier_tracking_number_author_id_key UNIQUE (carrier, tracking_number, author_id);


--
-- Name: shipment shipment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shipment
    ADD CONSTRAINT shipment_pkey PRIMARY KEY (id);


--
-- Name: user_karma user_karma_guild_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_karma
    ADD CONSTRAINT user_karma_guild_id_user_id_key UNIQUE (guild_id, user_id);


--
-- Name: user_money user_money_guild_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_money
    ADD CONSTRAINT user_money_guild_id_user_id_key UNIQUE (guild_id, user_id);


--
-- Name: user_presence user_presence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_presence
    ADD CONSTRAINT user_presence_pkey PRIMARY KEY (id);


--
-- Name: voice_state voice_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voice_state
    ADD CONSTRAINT voice_state_pkey PRIMARY KEY (id);


--
-- Name: vote vote_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vote
    ADD CONSTRAINT vote_pkey PRIMARY KEY (id);


--
-- Name: message_author_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_author_id_idx ON public.message USING btree (author_id);


--
-- Name: message_chan_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX message_chan_id_idx ON public.message USING btree (chan_id);


--
-- Name: user_presence_guild_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_presence_guild_id_idx ON public.user_presence USING btree (guild_id);


--
-- Name: user_presence_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX user_presence_user_id_idx ON public.user_presence USING btree (user_id);


--
-- Name: message message_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER message_update BEFORE UPDATE ON public.message FOR EACH ROW EXECUTE FUNCTION public.on_record_update();


--
-- Name: error_ip error_ip_error_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.error_ip
    ADD CONSTRAINT error_ip_error_id_fkey FOREIGN KEY (error_id) REFERENCES public.error(id);


--
-- Name: vote vote_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vote
    ADD CONSTRAINT vote_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.message(id);


--
-- Name: SCHEMA public; Type: ACL; Schema: -; Owner: -
--

GRANT USAGE ON SCHEMA public TO disgo;


--
-- Name: TABLE discord_quote; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.discord_quote TO disgo;


--
-- Name: SEQUENCE discord_quote_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.discord_quote_id_seq TO disgo;


--
-- Name: TABLE error; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.error TO disgo;


--
-- Name: TABLE error_ip; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.error_ip TO disgo;


--
-- Name: TABLE message; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.message TO disgo;


--
-- Name: TABLE own_username; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.own_username TO disgo;


--
-- Name: SEQUENCE own_username_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.own_username_id_seq TO disgo;


--
-- Name: TABLE pee_log; Type: ACL; Schema: public; Owner: -
--

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE public.pee_log TO disgo;


--
-- Name: TABLE reminder; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.reminder TO disgo;


--
-- Name: SEQUENCE reminder_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.reminder_id_seq TO disgo;


--
-- Name: TABLE shipment; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.shipment TO disgo;


--
-- Name: SEQUENCE shipment_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.shipment_id_seq TO disgo;


--
-- Name: TABLE user_karma; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.user_karma TO disgo;


--
-- Name: TABLE user_money; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.user_money TO disgo;


--
-- Name: TABLE user_presence; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.user_presence TO disgo;


--
-- Name: SEQUENCE user_presence_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.user_presence_id_seq TO disgo;


--
-- Name: TABLE voice_state; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.voice_state TO disgo;


--
-- Name: SEQUENCE voice_state_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.voice_state_id_seq TO disgo;


--
-- Name: TABLE vote; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON TABLE public.vote TO disgo;


--
-- Name: SEQUENCE vote_id_seq; Type: ACL; Schema: public; Owner: -
--

GRANT ALL ON SEQUENCE public.vote_id_seq TO disgo;


--
-- PostgreSQL database dump complete
--

